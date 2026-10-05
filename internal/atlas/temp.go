package atlas

import (
	"cmp"
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
	var size int64
	var newest time.Time
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
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
func (s *Service) scanTemps() error {
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
	for _, r := range refs {
		for _, root := range s.roots {
			if strings.HasPrefix(r.Path, root+string(os.PathSeparator)) {
				relative := strings.TrimPrefix(r.Path, root+string(os.PathSeparator))
				name, _, _ := strings.Cut(relative, string(os.PathSeparator))
				key := filepath.Join(root, name)
				if len(byPath[key]) < 50 {
					byPath[key] = append(byPath[key], r)
				}
			}
		}
	}
	out := []TempFile{}
	for i, root := range s.roots {
		s.progress("扫描临时目录 "+root, i, len(s.roots))
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
			n, newest, err := measure(p)
			row.Bytes = n
			if !newest.IsZero() {
				row.Modified = newest.Format(time.RFC3339)
			}
			if err != nil {
				row.Error = err.Error()
			}
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
