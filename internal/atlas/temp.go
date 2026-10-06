package atlas

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// measure does not follow symbolic links; permission failures remain visible to the user.
func measure(path string) (int64, time.Time, error) {
	return measureContext(context.Background(), path)
}
func measureContext(ctx context.Context, path string) (int64, time.Time, error) {
	var size int64
	var newest time.Time
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return size, newest, err
}
func (s *Service) scanTemps() error { return s.scanTempsContext(context.Background()) }
func (s *Service) scanTempsContext(ctx context.Context) error {
	roots := s.tempRoots()
	refs := []Evidence{}
	rows, err := s.db.Query("SELECT session_id,path,project,kind FROM refs")
	if err != nil {
		return err
	}
	for rows.Next() {
		var r Evidence
		if err = rows.Scan(&r.SessionID, &r.Path, &r.Project, &r.Kind); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Index references by each configured root's immediate child, avoiding O(files * refs).
	byPath := map[string][]Evidence{}
	owners := map[string]map[string]bool{}
	for _, r := range refs {
		for _, root := range roots {
			if strings.HasPrefix(r.Path, root+string(os.PathSeparator)) {
				relative := strings.TrimPrefix(r.Path, root+string(os.PathSeparator))
				name, _, _ := strings.Cut(relative, string(os.PathSeparator))
				key := filepath.Join(root, name)
				if owners[key] == nil {
					owners[key] = map[string]bool{}
				}
				owners[key][r.Project] = true
				if len(byPath[key]) < 50 {
					byPath[key] = append(byPath[key], r)
				}
			}
		}
	}
	out := []TempFile{}
	for i, root := range roots {
		s.progress("扫描临时目录 "+root, i, len(roots))
		entries, err := os.ReadDir(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			s.scanError(err)
			continue
		}
		for _, e := range entries {
			p := filepath.Join(root, e.Name())
			info, err := e.Info()
			if err != nil {
				s.scanError(err)
				continue
			}
			if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			row := TempFile{Path: p, Name: e.Name(), Mtime: info.ModTime().UnixNano(), Modified: info.ModTime().Format(time.RFC3339), IsDir: info.IsDir(), Link: info.Mode()&os.ModeSymlink != 0, Evidence: byPath[p]}
			if row.Evidence == nil {
				row.Evidence = []Evidence{}
			}
			if s.excluded(p) {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			n, newest, err := measureContext(ctx, p)
			row.Bytes = n
			if !newest.IsZero() {
				row.Modified = newest.Format(time.RFC3339)
			}
			if err != nil {
				row.Error = err.Error()
			}
			row.Shared = len(owners[p]) > 1
			classifyTemp(&row)
			out = append(out, row)
		}
	}
	slices.SortFunc(out, func(a, b TempFile) int { return cmp.Compare(b.Bytes, a.Bytes) })
	s.mu.Lock()
	s.temps = out
	s.mu.Unlock()
	return nil
}
func (s *Service) TempFiles() []TempFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.temps)
}

func classifyTemp(v *TempFile) {
	projects := map[string]bool{}
	created := false
	for _, e := range v.Evidence {
		projects[e.Project] = true
		if e.Kind == "创建操作记录" || e.Kind == "用户确认" {
			created = true
		}
	}
	v.Shared = v.Shared || len(projects) > 1 || slices.Contains([]string{"plugins", "bundled-marketplaces", "arg0", "node-compile-cache", "go-buildcache"}, v.Name)
	v.Confidence = "unknown"
	v.CleanupBlocked = "归属未知，请先查看依据并确认项目"
	if len(v.Evidence) > 0 {
		v.Confidence = "reference"
		v.CleanupBlocked = "只有路径引用，尚未确认归属"
	}
	if created {
		v.Confidence = "confirmed"
		v.CleanupBlocked = ""
	}
	if v.Shared {
		v.CleanupBlocked = "多项目引用或工具共享目录，禁止整体清理"
	}
	if v.Link {
		v.CleanupBlocked = "不处理符号链接"
	}
	if v.Error != "" {
		v.CleanupBlocked = "扫描不完整"
	}
}
func (s *Service) ConfirmTempProject(path, project string) error {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE project=?", project).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return errors.New("请选择已索引的项目")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.temps {
		v := &s.temps[i]
		if v.Path != path {
			continue
		}
		if v.Shared || v.Link || v.Error != "" {
			return errors.New("共享目录、符号链接或扫描不完整的项不能确认")
		}
		v.Evidence = append(v.Evidence, Evidence{Project: project, Path: path, Kind: "用户确认"})
		classifyTemp(v)
		return nil
	}
	return errors.New("请重新扫描临时目录")
}
