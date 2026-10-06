package atlas

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func randomID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func (s *Service) tempCandidate(path string) (TempFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.temps {
		if v.Path == path {
			return v, nil
		}
	}
	return TempFile{}, errors.New("请先扫描临时目录")
}
func (s *Service) validateTemp(path string) error { return s.validateTempForPlan(path, false) }
func (s *Service) validateTempForPlan(path string, force bool) error {
	v, err := s.tempCandidate(path)
	if err != nil {
		return err
	}
	if v.Link {
		return errors.New("不处理符号链接")
	}
	classifyTemp(&v)
	if v.CleanupBlocked != "" && !(force && v.Error == "") {
		return errors.New(v.CleanupBlocked)
	}
	if len(v.Evidence) == 0 && !force {
		return errors.New("归属未知，禁止批量清理")
	}
	if v.Error != "" {
		return errors.New("目录扫描不完整")
	}
	allowed := false
	for _, root := range s.tempRoots() {
		if filepath.Dir(path) == root && path != root {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("路径不在已扫描的临时目录内")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if canonicalTmp(resolved) != canonicalTmp(path) {
		return errors.New("路径含符号链接跳转")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	n, _, err := measure(path)
	if err != nil {
		return err
	}
	if info.ModTime().UnixNano() != v.Mtime || n != v.Bytes {
		return errors.New("扫描后文件发生变化，请重新扫描并确认归属")
	}
	return nil
}
func (s *Service) validateSession(v Session) error {
	if !uuidPattern.MatchString(v.ID) {
		return errors.New("无效会话 ID")
	}
	if v.Missing {
		return errors.New("原始日志已移除")
	}
	rel, err := filepath.Rel(s.homeFor(v), v.Path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(rel, "sessions"+string(os.PathSeparator)) && !strings.HasPrefix(rel, "archived_sessions"+string(os.PathSeparator)) {
		return errors.New("会话路径不在 Codex home 内")
	}
	p, err := filepath.EvalSymlinks(v.Path)
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(s.homeFor(v))
	if err != nil {
		return err
	}
	if p != filepath.Join(root, rel) {
		return errors.New("会话路径含符号链接跳转")
	}
	return nil
}
func checkIdle(path string) error {
	_, newest, err := measure(path)
	if err != nil {
		return err
	}
	if time.Since(newest) < 24*time.Hour {
		return errors.New("最近 24 小时有改动，暂不清理")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("不处理符号链接")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	flag := "--"
	args := []string{flag, path}
	if info.IsDir() {
		args = []string{"+D", path}
	}
	cmd := exec.CommandContext(ctx, "lsof", args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return errors.New("占用检查超时")
	}
	if len(out) > 0 {
		return errors.New("文件正被占用或无法完整检查")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("无法检查文件占用: %w", err)
	}
	return errors.New("无法确定文件是否空闲")
}
func (s *Service) PreviewClean(kind string, ids []string) (CleanPlan, error) {
	return s.previewClean(kind, ids, false)
}
func (s *Service) PreviewForceTempClean(ids []string) (CleanPlan, error) {
	return s.previewClean("temp", ids, true)
}
func (s *Service) previewClean(kind string, ids []string, force bool) (CleanPlan, error) {
	if kind != "session" && kind != "temp" {
		return CleanPlan{}, errors.New("未知清理类型")
	}
	if len(ids) == 0 || len(ids) > 50 {
		return CleanPlan{}, errors.New("每次请选择 1–50 项")
	}
	if !s.operation.TryLock() {
		return CleanPlan{}, errors.New("请等待当前扫描或操作完成")
	}
	defer s.operation.Unlock()
	plan := CleanPlan{Force: force, Backup: s.Settings().BackupSessions, Token: randomID(), Kind: kind, Items: []CleanItem{}, Expires: time.Now().Add(5 * time.Minute)}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		item := CleanItem{ID: id}
		var err error
		if kind == "session" {
			var v Session
			v, err = s.session(id)
			item.Path = v.Path
			if err == nil {
				err = s.validateSession(v)
			}
			if err == nil {
				_, err = s.cliBinary()
			}
			if err == nil {
				info, e := os.Stat(v.Path)
				err = e
				if e == nil && (info.Size() != v.Size || info.ModTime().UnixNano() != v.Mtime) {
					err = errors.New("会话已变化，请先重新扫描以保留最新统计")
				}
			}
		} else {
			item.Path = id
			err = s.validateTempForPlan(id, force)
			if runtime.GOOS != "darwin" {
				err = errors.New("此版本临时文件回收站仅支持 macOS")
			}
		}
		if err == nil {
			err = checkIdle(item.Path)
		}
		if err == nil {
			info, e := os.Lstat(item.Path)
			err = e
			if e == nil {
				item.Mtime = info.ModTime().UnixNano()
				item.Bytes, _, err = measure(item.Path)
			}
		}
		if err != nil {
			item.Blocked = err.Error()
		} else {
			plan.Bytes += item.Bytes
		}
		plan.Items = append(plan.Items, item)
	}
	s.mu.Lock()
	for token, p := range s.plans {
		if time.Now().After(p.Expires) {
			delete(s.plans, token)
		}
	}
	s.plans[plan.Token] = plan
	s.mu.Unlock()
	return plan, nil
}
func (s *Service) ExecuteClean(token, confirmation string) ([]CleanResult, error) {
	if confirmation != "确认清理" && confirmation != "确认强制清理" {
		return nil, errors.New("请输入“确认清理”")
	}
	if !s.operation.TryLock() {
		return nil, errors.New("请等待当前扫描或操作完成")
	}
	defer s.operation.Unlock()
	s.mu.Lock()
	plan, ok := s.plans[token]
	if ok && ((plan.Force && confirmation != "确认强制清理") || (!plan.Force && confirmation != "确认清理")) {
		s.mu.Unlock()
		return nil, errors.New("确认文字与清理模式不匹配，请核对预览")
	}
	delete(s.plans, token)
	s.mu.Unlock()
	if !ok || time.Now().After(plan.Expires) {
		return nil, errors.New("预览已过期，请重新生成")
	}
	results := []CleanResult{}
	for _, item := range plan.Items {
		r := CleanResult{ID: item.ID}
		err := func() error {
			if item.Blocked != "" {
				return errors.New(item.Blocked)
			}
			if plan.Kind == "temp" {
				if err := s.validateTempForPlan(item.Path, plan.Force); err != nil {
					return err
				}
			} else {
				v, err := s.session(item.ID)
				if err != nil {
					return err
				}
				if err = s.validateSession(v); err != nil {
					return err
				}
			}
			info, err := os.Lstat(item.Path)
			if err != nil {
				return err
			}
			n, _, err := measure(item.Path)
			if err != nil {
				return err
			}
			if info.ModTime().UnixNano() != item.Mtime || n != item.Bytes {
				return errors.New("预览后文件发生变化，请重新扫描")
			}
			if err = checkIdle(item.Path); err != nil {
				return err
			}
			if plan.Kind == "temp" {
				if err = s.trashWithRecovery(item); err != nil {
					return err
				}
				r.Message = "已移入废纸篓；清空废纸篓后释放空间"
				if plan.Force {
					r.Message = "已跳过归属标记检查；" + r.Message
				}
			} else if plan.Backup {
				v, err := s.session(item.ID)
				if err != nil {
					return err
				}
				if err = s.backupAndCleanSession(v); err != nil {
					return err
				}
				r.Message = "日志已归档并压缩备份，可在清理历史中恢复；备份仍占用部分空间"
			} else {
				v, err := s.session(item.ID)
				if err != nil {
					return err
				}
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				cmd, err := s.codexCommandHome(ctx, s.homeFor(v), "delete", "--force", item.ID)
				if err != nil {
					return err
				}
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("Codex 删除失败: %s (%w)", shortText(string(out), 800), err)
				}
				if _, err = os.Stat(item.Path); !errors.Is(err, os.ErrNotExist) {
					return errors.New("Codex 返回成功，但原日志仍存在；请重新扫描")
				}
				if _, err = s.db.Exec("UPDATE sessions SET missing=1 WHERE id=?", item.ID); err != nil {
					return err
				}
				r.Message = "会话已删除，历史用量统计已保留"
				if err = s.saveRecovery(Recovery{ID: randomID(), Kind: "permanent", Source: item.Path, SessionID: item.ID, Home: s.homeFor(v), Created: time.Now().Format(time.RFC3339Nano), State: "deleted", Size: item.Bytes}); err != nil {
					return err
				}
			}
			return nil
		}()
		r.Success = err == nil
		if err != nil {
			r.Message = err.Error()
		}
		results = append(results, r)
		if _, err = s.db.Exec("INSERT INTO audit VALUES(?,?,?,?)", time.Now().Format(time.RFC3339), plan.Kind, item.Path, r.Message); err != nil {
			return results, fmt.Errorf("写入操作记录失败: %w", err)
		}
	}
	err := s.refreshStorage(context.Background())
	if err != nil {
		return results, err
	}
	s.mu.Lock()
	s.temps = []TempFile{}
	s.mu.Unlock()
	return results, nil
}
func (s *Service) SetArchived(id string, archive bool) error {
	if !s.operation.TryLock() {
		return errors.New("请等待当前操作完成")
	}
	defer s.operation.Unlock()
	v, err := s.session(id)
	if err != nil {
		return err
	}
	if err = s.validateSession(v); err != nil {
		return err
	}
	verb := "archive"
	if !archive {
		verb = "unarchive"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd, err := s.codexCommandHome(ctx, s.homeFor(v), verb, id)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", shortText(string(out), 800), err)
	}
	return s.scan(false)
}
func (s *Service) ResumeCommand(id string) (string, error) {
	v, err := s.session(id)
	if err != nil {
		return "", err
	}
	if err = s.validateSession(v); err != nil {
		return "", err
	}
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
	return "CODEX_HOME=" + quote(s.homeFor(v)) + " codex -C " + quote(v.Project) + " resume " + quote(v.ID), nil
}
