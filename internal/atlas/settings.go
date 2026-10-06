package atlas

import (
	"ai-atlas/internal/buildinfo"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

type Settings struct {
	Home                string   `json:"home"`
	CLI                 string   `json:"cli"`
	ScanSystemTemp      bool     `json:"scanSystemTemp"`
	TempDirectories     []string `json:"tempDirectories"`
	ExcludedDirectories []string `json:"excludedDirectories"`
	BackupSessions      bool     `json:"backupSessions"`
	Configured          bool     `json:"configured"`
}
type EnvironmentStatus struct {
	HomeExists bool   `json:"homeExists"`
	CLIPath    string `json:"cliPath"`
	CLIVersion string `json:"cliVersion"`
	Error      string `json:"error"`
}

func sourceID(home string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(home)))
	return hex.EncodeToString(sum[:8])
}
func (s *Service) sourceHome() string { s.mu.Lock(); defer s.mu.Unlock(); return s.home }
func (s *Service) tempRoots() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.roots)
}
func (s *Service) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.settings
	v.Home = s.home
	v.TempDirectories = slices.Clone(v.TempDirectories)
	v.ExcludedDirectories = slices.Clone(v.ExcludedDirectories)
	return v
}
func (s *Service) SaveSettings(v Settings) error {
	if !s.operation.TryLock() {
		return errors.New("请先取消或等待当前扫描、清理操作完成")
	}
	defer s.operation.Unlock()
	home, err := normalizeDirectory(v.Home)
	if err != nil {
		return fmt.Errorf("Codex 目录: %w", err)
	}
	v.Home = home
	if v.CLI != "" {
		v.CLI, err = filepath.Abs(v.CLI)
		if err != nil {
			return err
		}
		info, e := os.Stat(v.CLI)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return errors.New("Codex CLI 必须是可执行文件")
		}
	}
	for _, list := range []*[]string{&v.TempDirectories, &v.ExcludedDirectories} {
		normalized := []string{}
		for _, p := range *list {
			if strings.TrimSpace(p) == "" {
				continue
			}
			p, err = normalizeDirectory(p)
			if err != nil {
				return err
			}
			userHome, _ := os.UserHomeDir()
			if p == string(os.PathSeparator) || (list == &v.TempDirectories && (p == canonicalTmp(userHome) || p == v.Home || p == filepath.Dir(s.dbPath))) {
				return errors.New("不能将文件系统根目录、用户目录或应用数据目录作为临时扫描根目录")
			}
			if !slices.Contains(normalized, p) {
				normalized = append(normalized, p)
			}
		}
		*list = normalized
	}
	v.Configured = true
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec("INSERT OR REPLACE INTO metadata VALUES('settings',?)", string(raw)); err != nil {
		return err
	}
	s.mu.Lock()
	s.settings = v
	s.home = v.Home
	s.roots = rootsFor(v)
	s.temps = []TempFile{}
	s.status = ScanStatus{Errors: []string{}, FailedFiles: []string{}}
	clear(s.plans)
	s.mu.Unlock()
	return nil
}
func normalizeDirectory(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("目录不能为空")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			p = user
		} else {
			p = filepath.Join(user, strings.TrimPrefix(p, "~/"))
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("所选路径不是目录")
	}
	return canonicalTmp(resolved), nil
}
func rootsFor(v Settings) []string {
	roots := []string{filepath.Join(v.Home, "tmp"), filepath.Join(v.Home, ".tmp")}
	if v.ScanSystemTemp {
		roots = append(roots, "/tmp", os.TempDir())
	}
	roots = append(roots, v.TempDirectories...)
	out := []string{}
	for _, p := range roots {
		p = canonicalTmp(p)
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
func (s *Service) excluded(path string) bool {
	v := s.Settings()
	path = canonicalTmp(path)
	for _, root := range v.ExcludedDirectories {
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
func (s *Service) Environment() EnvironmentStatus {
	v := s.Settings()
	out := EnvironmentStatus{}
	if st, err := os.Stat(v.Home); err == nil {
		out.HomeExists = st.IsDir()
	}
	path, err := s.cliBinary()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.CLIPath = path
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := s.codexCommand(ctx, "--version")
	if err != nil {
		out.Error = err.Error()
		return out
	}
	b, err := cmd.CombinedOutput()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.CLIVersion = strings.TrimSpace(string(b))
	return out
}
func (s *Service) cliBinary() (string, error) {
	v := s.Settings()
	if v.CLI != "" {
		return v.CLI, nil
	}
	return codexBinary()
}
func (s *Service) homeFor(v Session) string { return cmp.Or(v.SourceHome, s.sourceHome()) }

func AttachPickers(s *Service, dir, file func() (string, error)) {
	s.pickDirectory = dir
	s.pickFile = file
}
func (s *Service) ChooseDirectory() (string, error) {
	if s.pickDirectory == nil {
		return "", errors.New("当前运行方式不支持系统目录选择，请手动输入")
	}
	return s.pickDirectory()
}
func (s *Service) ChooseCLI() (string, error) {
	if s.pickFile == nil {
		return "", errors.New("当前运行方式不支持系统文件选择，请手动输入")
	}
	return s.pickFile()
}
func (s *Service) Diagnostics() (string, error) {
	v := s.Status()
	var sessions, events int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil {
		return "", err
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&events); err != nil {
		return "", err
	}
	// Diagnostics intentionally omit file paths, transcript text, titles, IDs, and raw errors.
	out := map[string]any{"version": buildinfo.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "provider": "codex", "sessions": sessions, "usageRecords": events, "scanRunning": v.Running, "scanErrors": len(v.Errors), "failedFiles": len(v.FailedFiles), "scanCancelled": v.Cancelled, "scanElapsedMillis": v.ElapsedMillis, "systemTempEnabled": s.Settings().ScanSystemTemp}
	b, err := json.MarshalIndent(out, "", "  ")
	return string(b), err
}

func InstanceKey(s *Service) string { return "local.aiatlas." + sourceID(s.dbPath) }
