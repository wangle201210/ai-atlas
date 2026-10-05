package atlas

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeLog(t *testing.T, home, id, parent string, events []map[string]any) string {
	t.Helper()
	path := filepath.Join(home, "sessions", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	meta := map[string]any{"type": "session_meta", "timestamp": "2026-01-01T00:00:00Z", "payload": map[string]any{"id": id, "cwd": "/projects/atlas", "timestamp": "2026-01-01T00:00:00Z", "forked_from_id": parent}}
	if err = enc.Encode(meta); err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if err = enc.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	return path
}
func ctx(turn, project string) map[string]any {
	return map[string]any{"type": "turn_context", "payload": map[string]any{"turn_id": turn, "cwd": project, "model": "test-model"}}
}
func tokens(total, last int64) map[string]any {
	return map[string]any{"type": "event_msg", "timestamp": "2026-01-02T12:00:00Z", "payload": map[string]any{"type": "token_count", "info": map[string]any{"total_token_usage": map[string]int64{"input_tokens": total - 10, "cached_input_tokens": max(0, total-20), "output_tokens": 10, "reasoning_output_tokens": 4, "total_tokens": total}, "last_token_usage": map[string]int64{"input_tokens": last - 10, "cached_input_tokens": max(0, last-20), "output_tokens": 10, "reasoning_output_tokens": 4, "total_tokens": last}}}}
}
func testService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "codex")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := New(home, filepath.Join(root, "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Close(s) })
	return s
}
func TestCumulativeUsageAndRepeatedSnapshots(t *testing.T) {
	home := t.TempDir()
	path := writeLog(t, home, "one", "", []map[string]any{ctx("turn-1", "/projects/one"), tokens(100, 100), tokens(100, 100), tokens(250, 150)})
	info, _ := os.Stat(path)
	p, err := parseSession(path, info)
	if err != nil {
		t.Fatal(err)
	}
	var sum Usage
	for _, e := range p.Events {
		addUsage(&sum, e.Usage)
	}
	if sum.Total != 250 || sum.Input != 240 || sum.Output != 10 || sum.Cached != 230 {
		t.Fatalf("unexpected totals: %+v", sum)
	}
}
func TestForkDedupAndRetainedLedger(t *testing.T) {
	s := testService(t)
	common := []map[string]any{ctx("shared-turn", "/projects/atlas"), tokens(100, 100), tokens(250, 150)}
	parent := writeLog(t, s.home, "00000000-0000-0000-0000-000000000001", "", common)
	writeLog(t, s.home, "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000001", append(common, ctx("child-turn", "/projects/atlas"), tokens(400, 150)))
	if err := Scan(s, false); err != nil {
		t.Fatal(err)
	}
	v, err := s.Overview("", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Usage.Total != 400 {
		t.Fatalf("fork double counted: %d", v.Usage.Total)
	}
	if err = os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err = Scan(s, false); err != nil {
		t.Fatal(err)
	}
	v, err = s.Overview("", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Usage.Total != 400 || v.Sessions != 2 {
		t.Fatalf("history not retained: %+v", v)
	}
}
func TestChangedFileReplacesIndexAndDateFilter(t *testing.T) {
	s := testService(t)
	writeLog(t, s.home, "id", "", []map[string]any{ctx("turn", "/projects/atlas"), tokens(100, 100)})
	if err := Scan(s, false); err != nil {
		t.Fatal(err)
	}
	writeLog(t, s.home, "id", "", []map[string]any{ctx("turn", "/projects/atlas"), tokens(100, 100), tokens(250, 150)})
	if err := Scan(s, false); err != nil {
		t.Fatal(err)
	}
	if err := Scan(s, false); err != nil {
		t.Fatal(err)
	}
	v, err := s.Overview("", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Usage.Total != 250 {
		t.Fatalf("unexpected total %d", v.Usage.Total)
	}
	v, err = s.Overview("2027-01-01", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Usage.Total != 0 {
		t.Fatal("date filter ignored")
	}
}
func TestOnlyToolReferencesAreEvidence(t *testing.T) {
	path := writeLog(t, t.TempDir(), "id", "", []map[string]any{ctx("t", "/projects/atlas"), {"type": "response_item", "payload": map[string]any{"type": "message", "role": "assistant", "content": []map[string]string{{"text": "/tmp/not-evidence"}}}}, {"type": "response_item", "payload": map[string]any{"type": "function_call", "arguments": `{"cmd":"mkdir /tmp/project-build"}`}}})
	info, _ := os.Stat(path)
	p, err := parseSession(path, info)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Refs) != 1 || p.Refs[0].Path != "/tmp/project-build" {
		t.Fatalf("unexpected refs %+v", p.Refs)
	}
}
func TestLargeRecordsAndPartialWrite(t *testing.T) {
	path := writeLog(t, t.TempDir(), "id", "", []map[string]any{{"type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []map[string]string{{"text": strings.Repeat("a", 100000)}}}}})
	info, _ := os.Stat(path)
	if _, err := parseSession(path, info); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"type":`)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parseSession(path, info); err == nil {
		t.Fatal("partial record accepted")
	}
}
func TestMeasureDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, make([]byte, 1000), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	n, _, err := measure(root)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("symlink was followed")
	}
}
func TestCleanupRejectsUnknownSymlinkAndExpiredPlan(t *testing.T) {
	s := testService(t)
	root := t.TempDir()
	s.roots = []string{root}
	p := filepath.Join(root, "unknown")
	if err := os.WriteFile(p, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	s.temps = []TempFile{{Path: p, Evidence: []Evidence{}}}
	if err := s.validateTemp(p); err == nil {
		t.Fatal("unknown ownership accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	s.temps = append(s.temps, TempFile{Path: link, Link: true, Evidence: []Evidence{{Path: link}}})
	if err := s.validateTemp(link); err == nil {
		t.Fatal("symlink accepted")
	}
	s.plans["expired"] = CleanPlan{Expires: time.Now().Add(-time.Second)}
	if _, err := s.ExecuteClean("expired", "确认清理"); err == nil {
		t.Fatal("expired plan accepted")
	}
	if _, err := s.ExecuteClean("anything", "wrong"); err == nil {
		t.Fatal("confirmation ignored")
	}
}
func TestCounterResetAndMissingPrefix(t *testing.T) {
	path := writeLog(t, t.TempDir(), "id", "", []map[string]any{ctx("t", "/projects/atlas"), tokens(1000, 100), tokens(1100, 100), tokens(50, 50)})
	info, _ := os.Stat(path)
	p, err := parseSession(path, info)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range p.Events {
		total += e.Usage.Total
	}
	if total != 250 {
		t.Fatalf("missing history or reset overcounted: %d", total)
	}
}

func TestTempScanAssociatesOnlyMatchingDirectoryBoundary(t *testing.T) {
	s := testService(t)
	root := t.TempDir()
	s.roots = []string{root}
	for _, name := range []string{"build", "build-other"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.db.Exec("INSERT INTO refs VALUES(?,?,?,?)", "session", filepath.Join(root, "build", "output.log"), "/projects/atlas", "命令引用")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.scanTemps(); err != nil {
		t.Fatal(err)
	}
	files := s.TempFiles()
	if len(files) != 2 {
		t.Fatalf("unexpected files %+v", files)
	}
	for _, f := range files {
		if f.Name == "build" && len(f.Evidence) != 1 {
			t.Fatal("association missing")
		}
		if f.Name == "build-other" && len(f.Evidence) != 0 {
			t.Fatal("prefix collision")
		}
	}
}

func TestSessionCleanupUsesCLIAndKeepsUsage(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof is required for occupancy verification")
	}
	s := testService(t)
	id := "00000000-0000-0000-0000-000000000009"
	path := writeLog(t, s.home, id, "", []map[string]any{ctx("delete-turn", "/projects/atlas"), tokens(100, 100)})
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := Scan(s, false); err != nil {
		t.Fatal(err)
	}
	// The fake CLI can only remove the fixture under CODEX_HOME, never real user sessions.
	cli := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\n[ \"$1\" = delete ] && [ \"$2\" = --force ] || exit 2\n/bin/rm -- \"$CODEX_HOME/sessions/$3.jsonl\"\n"
	if err := os.WriteFile(cli, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_ATLAS_CODEX", cli)
	plan, err := s.PreviewClean("session", []string{id})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Blocked != "" {
		t.Fatalf("unexpected blocked item: %s", plan.Items[0].Blocked)
	}
	result, err := s.ExecuteClean(plan.Token, "确认清理")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !result[0].Success {
		t.Fatalf("cleanup failed: %+v", result)
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture source still exists")
	}
	v, err := s.Overview("", "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Usage.Total != 100 || v.Bytes != 0 {
		t.Fatalf("ledger lost or size stale: %+v", v)
	}
	if _, err = s.ExecuteClean(plan.Token, "确认清理"); err == nil {
		t.Fatal("cleanup plan reused")
	}
}

func TestCustomToolCallPaths(t *testing.T) {
	path := writeLog(t, t.TempDir(), "custom", "", []map[string]any{ctx("turn", "/projects/custom"), {"type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "input": `await tools.exec_command({cmd:"mkdir /tmp/custom-build"})`}}, {"type": "response_item", "payload": map[string]any{"type": "custom_tool_call_output", "output": "Created /tmp/custom-build/output.txt"}}})
	info, _ := os.Stat(path)
	p, err := parseSession(path, info)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Refs) != 2 || p.Refs[0].Path != "/tmp/custom-build" || p.Refs[1].Path != "/tmp/custom-build/output.txt" {
		t.Fatalf("custom tool refs missing: %+v", p.Refs)
	}
}
