package atlas

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func waitScan(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for s.Status().Running {
		select {
		case <-deadline:
			t.Fatal("scan did not stop")
		case <-time.After(time.Millisecond):
		}
	}
}
func TestScanCancelRetryAndSettings(t *testing.T) {
	s := testService(t)
	path := writeLog(t, s.home, "cancel", "", []map[string]any{tokens(100, 100)})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("invalid\n")
	f.Close()
	if err = s.StartScan(false); err != nil {
		t.Fatal(err)
	}
	waitScan(t, s)
	if len(s.Status().FailedFiles) != 1 {
		t.Fatal("failed file not exposed")
	}
	writeLog(t, s.home, "cancel", "", []map[string]any{tokens(100, 100)})
	if err = s.RetryFailed(); err != nil {
		t.Fatal(err)
	}
	waitScan(t, s)
	if len(s.Status().Errors) > 0 {
		t.Fatal(s.Status().Errors)
	}
	if err = s.StartScan(false); err != nil {
		t.Fatal(err)
	}
	s.CancelScan()
	waitScan(t, s)
	if !s.Status().Cancelled {
		t.Fatal("cancellation not reflected")
	}
	cfg := s.Settings()
	if cfg.ScanSystemTemp || !cfg.BackupSessions {
		t.Fatal("unsafe initial settings")
	}
	cfg.ExcludedDirectories = []string{filepath.Dir(path)}
	if err = s.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if !s.Settings().Configured || !s.excluded(path) {
		t.Fatal("settings not applied")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = parseSessionContext(ctx, path, mustStat(t, path)); err == nil {
		t.Fatal("parser ignored cancellation")
	}
}
func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
func TestLatestMessagePagingAndSearch(t *testing.T) {
	s := testService(t)
	records := []map[string]any{ctx("turn", "/projects/atlas")}
	for i := 0; i < 121; i++ {
		records = append(records, map[string]any{"type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []map[string]string{{"text": strings.Repeat("x", i) + "needle"}}}})
	}
	writeLog(t, s.home, "messages", "", records)
	if err := Scan(s, false); err != nil {
		t.Fatal(err)
	}
	d, err := s.SessionMessages("messages", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.Total != 121 || len(d.Messages) != 50 || len(d.Messages[49].Text) != 126 {
		t.Fatalf("latest page incorrect: %+v", d)
	}
	d, err = s.SessionMessages("messages", strings.Repeat("x", 119), 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.Total != 2 || len(d.Messages) != 2 {
		t.Fatal("full text search incomplete")
	}
	d, err = s.SessionMessages("messages", "", 2)
	if err != nil || len(d.Messages) != 21 {
		t.Fatalf("older page failed: %+v %v", d, err)
	}
	if d.Session.Completeness != "none" || d.Session.HasUsage {
		t.Fatal("missing usage reported as zero")
	}
}
func TestReferenceConfidenceAndSharedDirectory(t *testing.T) {
	v := TempFile{Name: "build", Evidence: []Evidence{{Project: "p", Kind: "命令引用"}}}
	classifyTemp(&v)
	if v.CleanupBlocked == "" {
		t.Fatal("reference-only path allowed")
	}
	v.Evidence = append(v.Evidence, Evidence{Project: "p", Kind: "用户确认"})
	classifyTemp(&v)
	if v.CleanupBlocked != "" {
		t.Fatal("confirmed path blocked")
	}
	v.Evidence = append(v.Evidence, Evidence{Project: "other", Kind: "创建操作记录"})
	classifyTemp(&v)
	if !v.Shared || v.CleanupBlocked == "" {
		t.Fatal("shared root allowed")
	}
}
func TestCompressedSessionBackupAndRestore(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof needed")
	}
	s := testService(t)
	id := "00000000-0000-0000-0000-000000000010"
	path := writeLog(t, s.home, id, "", []map[string]any{ctx("restore-turn", "/projects/atlas"), tokens(100, 100)})
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(path, old, old)
	cli := filepath.Join(t.TempDir(), "codex")
	script := `#!/bin/sh
case "$1" in
archive) mkdir -p "$CODEX_HOME/archived_sessions"; mv "$CODEX_HOME/sessions/$2.jsonl" "$CODEX_HOME/archived_sessions/$2.jsonl";;
unarchive) mv "$CODEX_HOME/archived_sessions/$2.jsonl" "$CODEX_HOME/sessions/$2.jsonl";;
*) exit 2;;
esac
`
	if err = os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_ATLAS_CODEX", cli)
	if err = Scan(s, false); err != nil {
		t.Fatal(err)
	}
	plan, err := s.PreviewClean("session", []string{id})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Backup {
		t.Fatal("backup not default")
	}
	result, err := s.ExecuteClean(plan.Token, "确认清理")
	if err != nil || !result[0].Success {
		t.Fatalf("cleanup failed %+v %v", result, err)
	}
	history, err := s.Recoveries()
	if err != nil || len(history) != 1 {
		t.Fatalf("missing history %v", err)
	}
	r := history[0]
	if r.State != "ready" || r.StoredBytes <= 0 {
		t.Fatalf("invalid recovery %+v", r)
	}
	if err = s.RestoreRecovery(r.ID, "确认恢复"); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(path)
	if err != nil || string(restored) != string(original) {
		t.Fatal("restored log changed")
	}
	v, err := s.Overview("", "")
	if err != nil || v.Usage.Total != 100 {
		t.Fatalf("usage history duplicated/lost: %+v %v", v, err)
	}
}
func TestTempRestoreDoesNotOverwrite(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS trash integration")
	}
	s := testService(t)
	root := t.TempDir()
	s.trashDir = t.TempDir()
	path := filepath.Join(root, "artifact")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.trashWithRecovery(CleanItem{Path: path, Bytes: 8}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Recoveries()
	if err != nil {
		t.Fatal(err)
	}
	id := rows[0].ID
	if err = os.WriteFile(path, []byte("new file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreRecovery(id, "确认恢复"); err == nil {
		t.Fatal("overwrite allowed")
	}
	_ = os.Remove(path)
	if err = s.RestoreRecovery(id, "确认恢复"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "original" {
		t.Fatal("wrong restored content")
	}
}
func TestDiagnosticsRedactsPaths(t *testing.T) {
	s := testService(t)
	raw, err := s.Diagnostics()
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, s.home) || strings.Contains(raw, s.dbPath) {
		t.Fatal("diagnostics leaked paths")
	}
}

func TestObjectArgumentsAndPerFileRetryCache(t *testing.T) {
	s := testService(t)
	id := "argument-object"
	p := writeLog(t, s.home, id, "", []map[string]any{ctx("t", "/projects/atlas"), {"type": "response_item", "payload": map[string]any{"type": "function_call", "name": "exec_command", "arguments": map[string]string{"cmd": "mkdir /tmp/object-argument"}}}, tokens(100, 100)})
	if err := Scan(s, false); err != nil {
		t.Fatal(err)
	}
	v, err := s.session(id)
	if err != nil {
		t.Fatal(err)
	}
	if v.ParserVersion != "4" {
		t.Fatal("parser version not persisted")
	}
	// A stale per-file cache must be reparsed even though the global scan version is current.
	v.ParserVersion = "old"
	raw, _ := json.Marshal(v)
	if _, err = s.db.Exec("UPDATE sessions SET data=? WHERE id=?", string(raw), id); err != nil {
		t.Fatal(err)
	}
	if err = Scan(s, false); err != nil {
		t.Fatal(err)
	}
	v, err = s.session(id)
	if err != nil || v.ParserVersion != "4" {
		t.Fatalf("stale file cache skipped: %+v %v", v, err)
	}
	var n int
	if err = s.db.QueryRow("SELECT COUNT(*) FROM refs WHERE session_id=? AND path='/tmp/object-argument'", id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("object arguments not indexed: %d %v (%s)", n, err, p)
	}
}
