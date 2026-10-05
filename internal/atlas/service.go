package atlas

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type Service struct {
	db           *sql.DB
	home, dbPath string
	roots        []string
	mu           sync.Mutex
	status       ScanStatus
	temps        []TempFile
	plans        map[string]CleanPlan
	operation    sync.Mutex
}

func New(home, dbPath string) (*Service, error) {
	user, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	home = cmp.Or(home, os.Getenv("CODEX_HOME"), filepath.Join(user, ".codex"))
	home, err = filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	dbPath = cmp.Or(dbPath, os.Getenv("AI_ATLAS_DB"), filepath.Join(user, ".ai-atlas", "atlas.sqlite"))
	db, err := openStore(dbPath)
	if err != nil {
		return nil, err
	}
	roots := []string{}
	for _, p := range []string{filepath.Join(home, "tmp"), filepath.Join(home, ".tmp"), "/tmp", os.TempDir()} {
		p = canonicalTmp(p)
		if !slices.Contains(roots, p) {
			roots = append(roots, p)
		}
	}
	return &Service{db: db, home: home, dbPath: dbPath, roots: roots, status: ScanStatus{Errors: []string{}}, plans: map[string]CleanPlan{}, temps: []TempFile{}}, nil
}
func Close(s *Service) { s.operation.Lock(); defer s.operation.Unlock(); s.db.Close() }
func (s *Service) Status() ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.status
	v.Errors = slices.Clone(v.Errors)
	return v
}
func (s *Service) StartScan(includeTemps bool) error {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return errors.New("扫描正在进行")
	}
	s.status = ScanStatus{Running: true, Phase: "正在发现会话", Errors: []string{}}
	s.mu.Unlock()
	go func() {
		s.operation.Lock()
		defer s.operation.Unlock()
		err := s.scan(includeTemps)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.status.Errors = append(s.status.Errors, err.Error())
		}
		s.status.Running = false
		s.status.Phase = "扫描完成"
		s.status.Finished = time.Now().Format(time.RFC3339)
	}()
	return nil
}
func Scan(s *Service, includeTemps bool) error {
	s.operation.Lock()
	defer s.operation.Unlock()
	return s.scan(includeTemps)
}
func (s *Service) progress(phase string, done, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Phase = phase
	s.status.Done = done
	s.status.Total = total
}
func (s *Service) scanError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.status.Errors) < 30 {
		s.status.Errors = append(s.status.Errors, err.Error())
	}
}
func (s *Service) scan(includeTemps bool) error {
	const parserVersion = "2"
	var storedVersion string
	versionErr := s.db.QueryRow("SELECT value FROM metadata WHERE key='parserVersion'").Scan(&storedVersion)
	if versionErr != nil && !errors.Is(versionErr, sql.ErrNoRows) {
		return versionErr
	}
	files := []string{}
	for _, name := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(s.home, name)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() && strings.HasSuffix(path, ".jsonl") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	existing, err := s.allSessions()
	if err != nil {
		return err
	}
	byPath := map[string]Session{}
	for _, v := range existing {
		byPath[v.Path] = v
	}
	seenIDs := map[string]bool{}
	for i, path := range files {
		s.progress("索引会话日志", i, len(files))
		info, err := os.Stat(path)
		if err != nil {
			s.scanError(err)
			continue
		}
		if old, ok := byPath[path]; ok && storedVersion == parserVersion && old.Mtime == info.ModTime().UnixNano() && old.Size == info.Size() {
			seenIDs[old.ID] = true
			continue
		}
		p, err := parseSession(path, info)
		if err != nil {
			s.scanError(fmt.Errorf("%s: %w", filepath.Base(path), err))
			continue
		}
		// The active copy wins when a rollout also exists in archived_sessions.
		if seenIDs[p.Session.ID] {
			continue
		}
		seenIDs[p.Session.ID] = true
		if err = s.saveParsed(p); err != nil {
			return err
		}
	}
	for _, old := range existing {
		if _, err = os.Stat(old.Path); errors.Is(err, os.ErrNotExist) {
			if _, err = s.db.Exec("UPDATE sessions SET missing=1 WHERE id=? AND path=?", old.ID, old.Path); err != nil {
				return err
			}
		} else if err == nil {
			if _, err = s.db.Exec("UPDATE sessions SET missing=0 WHERE id=?", old.ID); err != nil {
				return err
			}
		}
	}
	s.progress("统计存储占用", len(files), len(files))
	storage := []Storage{}
	entries, err := os.ReadDir(s.home)
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := filepath.Join(s.home, e.Name())
		n, _, err := measure(p)
		row := Storage{Path: p, Bytes: n}
		if err != nil {
			row.Error = err.Error()
		}
		storage = append(storage, row)
	}
	slices.SortFunc(storage, func(a, b Storage) int { return cmp.Compare(b.Bytes, a.Bytes) })
	raw, _ := json.Marshal(storage)
	if _, err = s.db.Exec("INSERT OR REPLACE INTO metadata VALUES('storage',?)", string(raw)); err != nil {
		return err
	}
	if includeTemps {
		s.progress("分析临时文件及引用关系", 0, len(s.roots))
		if err = s.scanTemps(); err != nil {
			return err
		}
	}
	if _, err = s.db.Exec("INSERT OR REPLACE INTO metadata VALUES('parserVersion',?)", parserVersion); err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT OR REPLACE INTO metadata VALUES('lastScan',?)", time.Now().Format(time.RFC3339))
	return err
}
func (s *Service) saveParsed(p parsed) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	v := p.Session
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT OR REPLACE INTO sessions(id,path,project,created,size,mtime,missing,data) VALUES(?,?,?,?,?,?,0,?)", v.ID, v.Path, v.Project, v.Created, v.Size, v.Mtime, string(raw))
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM events WHERE session_id=?", v.ID); err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT OR IGNORE INTO events VALUES(?,?,?,?,?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range p.Events {
		u := e.Usage
		if _, err = stmt.Exec(e.Key, v.ID, e.Project, e.Day, u.Input, u.Cached, u.Output, u.Reasoning, u.Total); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("DELETE FROM refs WHERE session_id=?", v.ID); err != nil {
		return err
	}
	ref, err := tx.Prepare("INSERT OR IGNORE INTO refs VALUES(?,?,?,?)")
	if err != nil {
		return err
	}
	defer ref.Close()
	for _, r := range p.Refs {
		if _, err = ref.Exec(v.ID, r.Path, r.Project, r.Kind); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Service) Overview(since, until string) (Snapshot, error) {
	result := Snapshot{Home: s.home, Database: s.dbPath, TempRoots: s.roots, Projects: []Project{}, Days: []Day{}, Storage: []Storage{}}
	all, err := s.allSessions()
	if err != nil {
		return result, err
	}
	projects := map[string]*Project{}
	ensure := func(path string) *Project {
		if projects[path] == nil {
			projects[path] = &Project{Path: path, Name: filepath.Base(path)}
		}
		return projects[path]
	}
	for _, v := range all {
		p := ensure(v.Project)
		p.Sessions++
		result.Sessions++
		if !v.Missing {
			p.Bytes += v.Size
			result.Bytes += v.Size
		}
	}
	rows, err := s.db.Query("SELECT project,SUM(input),SUM(cached),SUM(output),SUM(reasoning),SUM(total) FROM unique_events WHERE (?='' OR day>=?) AND (?='' OR day<=?) GROUP BY project", since, since, until, until)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var path string
		var u Usage
		if err = rows.Scan(&path, &u.Input, &u.Cached, &u.Output, &u.Reasoning, &u.Total); err != nil {
			rows.Close()
			return result, err
		}
		ensure(path).Usage = u
		addUsage(&result.Usage, u)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	for _, p := range projects {
		result.Projects = append(result.Projects, *p)
	}
	slices.SortFunc(result.Projects, func(a, b Project) int { return cmp.Compare(b.Usage.Total, a.Usage.Total) })
	rows, err = s.db.Query("SELECT day,SUM(total) FROM unique_events WHERE (?='' OR day>=?) AND (?='' OR day<=?) GROUP BY day ORDER BY day DESC LIMIT 30", since, since, until, until)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var d Day
		if err = rows.Scan(&d.Date, &d.Total); err != nil {
			rows.Close()
			return result, err
		}
		result.Days = append(result.Days, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	slices.Reverse(result.Days)
	var raw string
	err = s.db.QueryRow("SELECT value FROM metadata WHERE key='storage'").Scan(&raw)
	if err == nil {
		if err = json.Unmarshal([]byte(raw), &result.Storage); err != nil {
			return result, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	err = s.db.QueryRow("SELECT value FROM metadata WHERE key='lastScan'").Scan(&result.LastScan)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	return result, nil
}
func (s *Service) Sessions(q SessionQuery) (SessionPage, error) {
	all, err := s.allSessions()
	if err != nil {
		return SessionPage{}, err
	}
	filtered := []Session{}
	search := strings.ToLower(q.Search)
	for _, v := range all {
		if q.Project != "" && v.Project != q.Project {
			continue
		}
		if !strings.Contains(strings.ToLower(v.Title+v.Project+v.ID), search) {
			continue
		}
		if q.State == "active" && (v.Archived || v.Missing) {
			continue
		}
		if q.State == "archived" && (!v.Archived || v.Missing) {
			continue
		}
		if q.State == "missing" && !v.Missing {
			continue
		}
		filtered = append(filtered, v)
	}
	slices.SortFunc(filtered, func(a, b Session) int {
		switch q.Sort {
		case "size":
			return cmp.Compare(b.Size, a.Size)
		case "tokens":
			return cmp.Compare(b.Usage.Total, a.Usage.Total)
		default:
			return strings.Compare(b.Updated, a.Updated)
		}
	})
	start := min(max(q.Page, 0)*50, len(filtered))
	return SessionPage{filtered[start:min(start+50, len(filtered))], len(filtered)}, nil
}

var stopPreview = errors.New("preview limit")

func (s *Service) SessionDetail(id string) (Detail, error) {
	v, err := s.session(id)
	if err != nil {
		return Detail{}, err
	}
	out := Detail{Session: v, Messages: []Message{}}
	if v.Missing {
		return out, nil
	}
	info, err := os.Stat(v.Path)
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() {
		return out, errors.New("会话文件不是普通文件")
	}
	err = readRecords(v.Path, func(r record, p payload) error {
		if r.Type != "response_item" || p.Type != "message" || (p.Role != "user" && p.Role != "assistant") {
			return nil
		}
		text := ""
		for _, c := range p.Content {
			if c.Text != "" {
				text += c.Text + "\n"
			}
		}
		if text != "" {
			out.Messages = append(out.Messages, Message{p.Role, shortText(text, 12000), r.Timestamp})
		}
		if len(out.Messages) >= 150 {
			out.Truncated = true
			return stopPreview
		}
		return nil
	})
	if errors.Is(err, stopPreview) {
		err = nil
	}
	return out, err
}
