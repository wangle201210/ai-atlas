package atlas

import (
	"cmp"
	"context"
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
	pickDirectory      func() (string, error)
	pickFile           func() (string, error)
	pickDiagnosticFile func() (string, error)
	pickReportFile     func() (string, error)
	settings           Settings
	scanCancel         context.CancelFunc
	jobs               sync.WaitGroup
	closing            bool
	trashDir           string
	db                 *sql.DB
	home, dbPath       string
	roots              []string
	mu                 sync.Mutex
	status             ScanStatus
	temps              []TempFile
	plans              map[string]CleanPlan
	operation          sync.Mutex
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
	dbPath, err = filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	db, err := openStore(dbPath)
	if err != nil {
		return nil, err
	}
	cfg := Settings{Home: home, ScanSystemTemp: true, BackupSessions: true, TempDirectories: []string{}, ExcludedDirectories: []string{}}
	var raw string
	if err = db.QueryRow("SELECT value FROM metadata WHERE key='settings'").Scan(&raw); err == nil {
		if err = json.Unmarshal([]byte(raw), &cfg); err != nil {
			db.Close()
			return nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		db.Close()
		return nil, err
	}
	if os.Getenv("CODEX_HOME") != "" {
		cfg.Home = home
	}
	return &Service{db: db, home: cfg.Home, dbPath: dbPath, roots: rootsFor(cfg), settings: cfg, trashDir: filepath.Join(user, ".Trash"), status: ScanStatus{Errors: []string{}, FailedFiles: []string{}}, plans: map[string]CleanPlan{}, temps: []TempFile{}}, nil
}
func Close(s *Service) {
	s.mu.Lock()
	s.closing = true
	if s.scanCancel != nil {
		s.scanCancel()
	}
	s.mu.Unlock()
	s.jobs.Wait()
	s.operation.Lock()
	defer s.operation.Unlock()
	s.db.Close()
}
func (s *Service) Status() ScanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.status
	v.Errors = slices.Clone(v.Errors)
	v.FailedFiles = slices.Clone(v.FailedFiles)
	if v.Running {
		if t, e := time.Parse(time.RFC3339Nano, v.Started); e == nil {
			v.ElapsedMillis = time.Since(t).Milliseconds()
		}
	}
	return v
}
func (s *Service) StartScan(includeTemps bool) error { return s.startScan(includeTemps, nil) }
func (s *Service) CancelScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scanCancel != nil {
		s.status.Phase = "正在取消扫描"
		s.scanCancel()
	}
}
func (s *Service) RetryFailed() error {
	s.mu.Lock()
	files := slices.Clone(s.status.FailedFiles)
	s.mu.Unlock()
	if len(files) == 0 {
		return errors.New("没有需要重试的日志文件")
	}
	return s.startScan(false, files)
}
func (s *Service) startScan(includeTemps bool, files []string) error {
	if !s.operation.TryLock() {
		return errors.New("请等待当前操作完成")
	}
	s.mu.Lock()
	if s.closing || s.status.Running {
		s.mu.Unlock()
		s.operation.Unlock()
		return errors.New("扫描正在进行或应用正在退出")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.scanCancel = cancel
	s.status = ScanStatus{Running: true, Phase: "正在发现会话", Started: time.Now().Format(time.RFC3339Nano), Errors: []string{}, FailedFiles: []string{}}
	s.jobs.Go(func() {
		defer cancel()
		defer s.operation.Unlock()
		err := s.scanContext(ctx, includeTemps, files)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.status.Running = false
		s.scanCancel = nil
		s.status.Phase = "扫描完成"
		s.status.Finished = time.Now().Format(time.RFC3339Nano)
		start, _ := time.Parse(time.RFC3339Nano, s.status.Started)
		s.status.ElapsedMillis = time.Since(start).Milliseconds()
		if errors.Is(err, context.Canceled) {
			s.status.Cancelled = true
			s.status.Phase = "已取消，已完成的索引保留"
		} else if err != nil {
			s.status.Phase = "扫描未完成"
			s.status.Errors = append(s.status.Errors, err.Error())
		}
	})
	s.mu.Unlock()
	return nil
}
func Scan(s *Service, includeTemps bool) error {
	s.operation.Lock()
	defer s.operation.Unlock()
	return s.scan(includeTemps)
}
func (s *Service) scan(includeTemps bool) error {
	return s.scanContext(context.Background(), includeTemps, nil)
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
func (s *Service) scanContext(ctx context.Context, includeTemps bool, retry []string) error {
	files := slices.Clone(retry)
	if len(retry) == 0 {
		for _, name := range []string{"sessions", "archived_sessions"} {
			root := filepath.Join(s.sourceHome(), name)
			err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if s.excluded(path) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
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
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.excluded(path) {
			continue
		}
		s.mu.Lock()
		s.status.CurrentFile = path
		s.mu.Unlock()
		s.progress("索引会话日志", i, len(files))
		info, err := os.Stat(path)
		if err != nil {
			s.scanError(err)
			continue
		}
		if old, ok := byPath[path]; ok && old.ParserVersion == parserVersion && old.Mtime == info.ModTime().UnixNano() && old.Size == info.Size() {
			seenIDs[old.ID] = true
			s.mu.Lock()
			s.status.Skipped++
			s.mu.Unlock()
			continue
		}
		p, err := parseSessionContext(ctx, path, info)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			s.mu.Lock()
			s.status.FailedFiles = append(s.status.FailedFiles, path)
			s.mu.Unlock()
			s.scanError(fmt.Errorf("%s: %w", filepath.Base(path), err))
			continue
		}
		// The active copy wins when a rollout also exists in archived_sessions.
		if seenIDs[p.Session.ID] {
			continue
		}
		seenIDs[p.Session.ID] = true
		p.Session.ParserVersion = parserVersion
		p.Session.Provider = "codex"
		p.Session.SourceHome = s.sourceHome()
		p.Session.SourceID = sourceID(s.sourceHome())
		p.Session.Key = "codex:" + p.Session.SourceID + ":" + p.Session.ID
		s.mu.Lock()
		s.status.Parsed++
		s.mu.Unlock()
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
	if err = s.refreshStorage(ctx); err != nil {
		return err
	}
	if includeTemps {
		s.progress("分析临时文件及引用关系", 0, len(s.tempRoots()))
		if err = s.scanTempsContext(ctx); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
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
	if _, err = tx.Exec("DELETE FROM messages WHERE session_id=?", v.ID); err != nil {
		return err
	}
	msg, err := tx.Prepare("INSERT INTO messages VALUES(?,?,?,?,?,?)")
	if err != nil {
		return err
	}
	defer msg.Close()
	for i, m := range p.Messages {
		if _, err = msg.Exec(v.ID, i, m.Offset, m.Length, m.Role, m.Time); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Service) Overview(since, until string) (Snapshot, error) {
	result := Snapshot{Home: s.sourceHome(), Database: s.dbPath, TempRoots: s.tempRoots(), Projects: []Project{}, Days: []Day{}, Storage: []Storage{}}
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
		switch v.Completeness {
		case "complete":
			result.CompleteSessions++
		case "none":
			result.NoUsageSessions++
		default:
			result.PartialSessions++
		}
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
	groups := map[string]map[string]bool{}
	for _, v := range all {
		if groups[v.Project] == nil {
			groups[v.Project] = map[string]bool{}
		}
		groups[v.Project][v.ID] = true
	}
	links, e := s.db.Query("SELECT DISTINCT project,session_id FROM events")
	if e != nil {
		return result, e
	}
	for links.Next() {
		var path, id string
		if e = links.Scan(&path, &id); e != nil {
			links.Close()
			return result, e
		}
		if groups[path] == nil {
			groups[path] = map[string]bool{}
		}
		groups[path][id] = true
	}
	e = links.Err()
	links.Close()
	if e != nil {
		return result, e
	}
	for path, p := range projects {
		p.Sessions = len(groups[path])
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
	related := map[string]bool{}
	if q.Project != "" {
		rows, e := s.db.Query("SELECT DISTINCT session_id FROM events WHERE project=?", q.Project)
		if e != nil {
			return SessionPage{}, e
		}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return SessionPage{}, e
			}
			related[id] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return SessionPage{}, e
		}
	}
	filtered := []Session{}
	search := strings.ToLower(q.Search)
	for _, v := range all {
		if q.Project != "" && v.Project != q.Project && !related[v.ID] {
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

func (s *Service) SessionDetail(id string) (Detail, error) { return s.SessionMessages(id, "", 0) }
