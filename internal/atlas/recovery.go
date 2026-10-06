package atlas

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Recovery struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Stored      string `json:"stored"`
	SessionID   string `json:"sessionId"`
	Home        string `json:"home"`
	Created     string `json:"created"`
	State       string `json:"state"`
	Error       string `json:"error"`
	RestoredAt  string `json:"restoredAt"`
	Size        int64  `json:"size"`
	StoredBytes int64  `json:"storedBytes"`
	Digest      string `json:"digest"`
	WasArchived bool   `json:"wasArchived"`
}

func (s *Service) saveRecovery(r Recovery) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT OR REPLACE INTO recovery VALUES(?,?)", r.ID, string(b))
	return err
}
func (s *Service) Recoveries() ([]Recovery, error) {
	rows, err := s.db.Query("SELECT data FROM recovery ORDER BY json_extract(data,'$.created') DESC LIMIT 300")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recovery{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r Recovery
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *Service) recovery(id string) (Recovery, error) {
	var raw string
	if err := s.db.QueryRow("SELECT data FROM recovery WHERE id=?", id).Scan(&raw); err != nil {
		return Recovery{}, err
	}
	var r Recovery
	err := json.Unmarshal([]byte(raw), &r)
	return r, err
}
func compressBackup(src, dst string) (digest string, size int64, err error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", 0, err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(dst)
		}
	}()
	h := sha256.New()
	z := gzip.NewWriter(out)
	if _, err = io.Copy(z, io.TeeReader(in, h)); err != nil {
		return "", 0, err
	}
	if err = z.Close(); err != nil {
		return "", 0, err
	}
	if err = out.Sync(); err != nil {
		return "", 0, err
	}
	info, err := out.Stat()
	if err != nil {
		return "", 0, err
	}
	ok = true
	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}
func digestFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), err
}
func findRollout(home, id string, archived bool) (string, error) {
	dir := "sessions"
	if archived {
		dir = "archived_sessions"
	}
	var found string
	err := filepath.WalkDir(filepath.Join(home, dir), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".jsonl") && strings.Contains(d.Name(), id) {
			if found != "" {
				return errors.New("发现多个同 ID 日志，停止操作")
			}
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("无法定位会话日志")
	}
	return found, nil
}
func (s *Service) updateSessionFile(v Session, path string, archived, missing bool) error {
	v.Path = path
	v.Archived = archived
	v.Missing = missing
	if !missing {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		v.Size = info.Size()
		v.Mtime = info.ModTime().UnixNano()
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE sessions SET path=?,size=?,mtime=?,missing=?,data=? WHERE id=?", path, v.Size, v.Mtime, missing, string(raw), v.ID)
	return err
}
func (s *Service) backupAndCleanSession(v Session) (err error) {
	root := filepath.Join(filepath.Dir(s.dbPath), "backups")
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	r := Recovery{ID: randomID(), Kind: "session", Source: v.Path, SessionID: v.ID, Home: s.homeFor(v), Created: time.Now().Format(time.RFC3339Nano), State: "preparing", Size: v.Size, WasArchived: v.Archived}
	r.Stored = filepath.Join(root, r.ID+".jsonl.gz")
	r.Digest, r.StoredBytes, err = compressBackup(v.Path, r.Stored)
	if err != nil {
		return err
	}
	if err = s.saveRecovery(r); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			r.State = "error"
			r.Error = err.Error()
			_ = s.saveRecovery(r)
		}
	}()
	path := v.Path
	if !v.Archived {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd, e := s.codexCommandHome(ctx, r.Home, "archive", v.ID)
		if e != nil {
			return e
		}
		if out, e := cmd.CombinedOutput(); e != nil {
			return fmt.Errorf("归档失败：%s (%w)", shortText(string(out), 500), e)
		}
		path, err = findRollout(r.Home, v.ID, true)
		if err != nil {
			return err
		}
	}
	r.Source = path
	if err = s.saveRecovery(r); err != nil {
		return err
	}
	digest, e := digestFile(path)
	if e != nil {
		return e
	}
	if digest != r.Digest {
		return errors.New("归档后日志内容变化，已保留备份但没有移除原文件")
	}
	if err = s.updateSessionFile(v, path, true, false); err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		return err
	}
	r.State = "ready"
	if err = s.saveRecovery(r); err != nil {
		return err
	}
	return s.updateSessionFile(v, path, true, true)
}
func (s *Service) trashWithRecovery(item CleanItem) error {
	if err := os.MkdirAll(s.trashDir, 0700); err != nil {
		return err
	}
	r := Recovery{ID: randomID(), Kind: "temp", Source: item.Path, Created: time.Now().Format(time.RFC3339Nano), State: "preparing", Size: item.Bytes}
	r.Stored = filepath.Join(s.trashDir, "ai-atlas-"+r.ID+"-"+filepath.Base(item.Path))
	if err := s.saveRecovery(r); err != nil {
		return err
	}
	if err := renameExclusive(item.Path, r.Stored); err != nil {
		r.State = "error"
		r.Error = err.Error()
		_ = s.saveRecovery(r)
		return err
	}
	r.State = "ready"
	return s.saveRecovery(r)
}
func (s *Service) RestoreRecovery(id, confirmation string) error {
	if confirmation != "确认恢复" {
		return errors.New("请输入“确认恢复”")
	}
	if !s.operation.TryLock() {
		return errors.New("请等待当前操作完成")
	}
	defer s.operation.Unlock()
	r, err := s.recovery(id)
	if err != nil {
		return err
	}
	if r.State == "restored" {
		return errors.New("此项已恢复")
	}
	if r.State != "ready" && r.State != "error" {
		return errors.New("此项尚未完成清理")
	}
	if _, err = os.Lstat(r.Source); err == nil {
		return errors.New("原路径已存在，拒绝覆盖")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(r.Source))
	if err != nil {
		return fmt.Errorf("原目录不存在或不可访问：%w", err)
	}
	if canonicalTmp(parent) != canonicalTmp(filepath.Dir(r.Source)) {
		return errors.New("原目录存在符号链接跳转，停止恢复")
	}
	stored, err := filepath.EvalSymlinks(r.Stored)
	if err != nil {
		return err
	}
	if canonicalTmp(stored) != canonicalTmp(r.Stored) {
		return errors.New("备份路径含符号链接")
	}
	if r.Kind == "temp" {
		if filepath.Dir(r.Stored) != s.trashDir {
			return errors.New("废纸篓路径不匹配")
		}
		if err = renameExclusive(r.Stored, r.Source); err != nil {
			return err
		}
	} else if r.Kind == "session" {
		root := filepath.Join(filepath.Dir(s.dbPath), "backups")
		if filepath.Dir(r.Stored) != root {
			return errors.New("备份路径不匹配")
		}
		rel, e := filepath.Rel(r.Home, r.Source)
		if e != nil || !strings.HasPrefix(rel, "archived_sessions"+string(os.PathSeparator)) {
			return errors.New("恢复路径不在归档日志目录内")
		}
		if err = restoreGzip(r.Stored, r.Source, r.Digest, r.Size); err != nil {
			return err
		}
		v, e := s.session(r.SessionID)
		if e != nil {
			return e
		}
		target := r.Source
		if !r.WasArchived {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd, e := s.codexCommandHome(ctx, r.Home, "unarchive", r.SessionID)
			if e != nil {
				return e
			}
			if out, e := cmd.CombinedOutput(); e != nil {
				_ = s.updateSessionFile(v, target, true, false)
				return fmt.Errorf("日志已恢复到归档目录，但取消归档失败：%s (%w)", shortText(string(out), 500), e)
			}
			target, err = findRollout(r.Home, r.SessionID, false)
			if err != nil {
				return err
			}
		}
		if err = s.updateSessionFile(v, target, r.WasArchived, false); err != nil {
			return err
		}
	} else {
		return errors.New("不支持的恢复类型")
	}
	r.State = "restored"
	r.RestoredAt = time.Now().Format(time.RFC3339Nano)
	if err = s.saveRecovery(r); err != nil {
		return err
	}
	return s.refreshStorage(context.Background())
}
func restoreGzip(src, dst, want string, maxBytes int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	z, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer z.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(dst)
		}
	}()
	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(out, h), io.LimitReader(z, maxBytes+1)); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("备份校验失败，已取消恢复")
	}
	if err = out.Sync(); err != nil {
		return err
	}
	ok = true
	return nil
}
