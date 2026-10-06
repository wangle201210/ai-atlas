package atlas

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path/filepath"
)

func openStore(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
 CREATE TABLE IF NOT EXISTS sessions(id TEXT PRIMARY KEY,path TEXT NOT NULL,project TEXT NOT NULL,created TEXT NOT NULL,size INTEGER NOT NULL,mtime INTEGER NOT NULL,missing INTEGER NOT NULL DEFAULT 0,data TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS events(key TEXT NOT NULL,session_id TEXT NOT NULL,project TEXT NOT NULL,day TEXT NOT NULL,input INTEGER NOT NULL,cached INTEGER NOT NULL,output INTEGER NOT NULL,reasoning INTEGER NOT NULL,total INTEGER NOT NULL,PRIMARY KEY(key,session_id));
 CREATE INDEX IF NOT EXISTS events_session ON events(session_id);
 CREATE TABLE IF NOT EXISTS refs(session_id TEXT NOT NULL,path TEXT NOT NULL,project TEXT NOT NULL,kind TEXT NOT NULL,PRIMARY KEY(session_id,path,project,kind));
 CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS messages(session_id TEXT NOT NULL,seq INTEGER NOT NULL,offset INTEGER NOT NULL,length INTEGER NOT NULL,role TEXT NOT NULL,time TEXT NOT NULL,PRIMARY KEY(session_id,seq));
 CREATE TABLE IF NOT EXISTS recovery(id TEXT PRIMARY KEY,data TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS audit(time TEXT NOT NULL,kind TEXT NOT NULL,target TEXT NOT NULL,result TEXT NOT NULL);
 CREATE VIEW IF NOT EXISTS unique_events AS SELECT key,session_id,project,day,input,cached,output,reasoning,total FROM (
 SELECT e.*,ROW_NUMBER() OVER(PARTITION BY e.key ORDER BY s.created,s.id) AS rank FROM events e JOIN sessions s ON s.id=e.session_id) WHERE rank=1;
 `)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Service) allSessions() ([]Session, error) {
	rows, err := s.db.Query("SELECT data,missing FROM sessions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var raw string
		var missing bool
		if err = rows.Scan(&raw, &missing); err != nil {
			return nil, err
		}
		var v Session
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		v.Missing = missing
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	usages := map[string]Usage{}
	rows, err = s.db.Query("SELECT session_id,SUM(input),SUM(cached),SUM(output),SUM(reasoning),SUM(total) FROM unique_events GROUP BY session_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var u Usage
		if err = rows.Scan(&id, &u.Input, &u.Cached, &u.Output, &u.Reasoning, &u.Total); err != nil {
			return nil, err
		}
		usages[id] = u
	}
	for i := range out {
		out[i].Usage = usages[out[i].ID]
		if out[i].Completeness == "" {
			out[i].Completeness = "partial"
			out[i].Warning = "请刷新索引以检测统计完整性"
			out[i].HasUsage = out[i].Usage.Total > 0
		}
	}
	return out, rows.Err()
}
func (s *Service) session(id string) (Session, error) {
	var raw string
	var missing bool
	err := s.db.QueryRow("SELECT data,missing FROM sessions WHERE id=?", id).Scan(&raw, &missing)
	if err != nil {
		return Session{}, fmt.Errorf("会话不存在: %w", err)
	}
	var v Session
	err = json.Unmarshal([]byte(raw), &v)
	v.Missing = missing
	return v, err
}
func addUsage(a *Usage, b Usage) {
	a.Input += b.Input
	a.Cached += b.Cached
	a.Output += b.Output
	a.Reasoning += b.Reasoning
	a.Total += b.Total
}
