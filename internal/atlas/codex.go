package atlas

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Finder applications do not inherit the user's interactive shell PATH.
func codexBinary() (string, error) {
	if path := os.Getenv("AI_ATLAS_CODEX"); path != "" {
		if filepath.IsAbs(path) {
			return path, nil
		}
		return "", errors.New("AI_ATLAS_CODEX 必须是绝对路径")
	}
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	paths := []string{"/opt/homebrew/bin/codex", "/usr/local/bin/codex", filepath.Join(home, ".local/bin/codex")}
	nvm, _ := filepath.Glob(filepath.Join(home, ".nvm/versions/node/*/bin/codex"))
	slices.Reverse(nvm)
	paths = append(paths, nvm...)
	for _, path := range paths {
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("未找到 Codex CLI，请安装或设置 AI_ATLAS_CODEX")
}
func (s *Service) codexCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	return s.codexCommandHome(ctx, s.sourceHome(), args...)
}
func (s *Service) codexCommandHome(ctx context.Context, home string, args ...string) (*exec.Cmd, error) {
	path, err := s.cliBinary()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "CODEX_HOME=") && !strings.HasPrefix(v, "PATH=") {
			env = append(env, v)
		}
	}
	cmd.Env = append(env, "CODEX_HOME="+home, "PATH="+filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return cmd, nil
}
