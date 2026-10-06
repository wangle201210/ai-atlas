package atlas

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

func (s *Service) refreshStorage(ctx context.Context) error {
	home := s.sourceHome()
	entries, err := os.ReadDir(home)
	if err != nil {
		return err
	}
	rows := []Storage{}
	for _, e := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(home, e.Name())
		if s.excluded(path) {
			continue
		}
		n, _, err := measureContext(ctx, path)
		row := Storage{Path: path, Bytes: n}
		if err != nil {
			row.Error = err.Error()
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b Storage) int { return cmp.Compare(b.Bytes, a.Bytes) })
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT OR REPLACE INTO metadata VALUES('storage',?)", string(raw))
	return err
}
