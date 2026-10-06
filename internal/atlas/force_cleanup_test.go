package atlas

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestForceTempCleanupKeepsGuards(t *testing.T) {
	s := testService(t)
	root := t.TempDir()
	s.roots = []string{root}
	p := filepath.Join(root, "unattributed")
	if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	st := mustStat(t, p)
	candidate := TempFile{Path: p, Name: "unattributed", Bytes: st.Size(), Mtime: st.ModTime().UnixNano()}
	s.temps = []TempFile{candidate}
	if s.validateTemp(p) == nil {
		t.Fatal("ordinary mode must require ownership")
	}
	if err := s.validateTempForPlan(p, true); err != nil {
		t.Fatal(err)
	}
	shared := candidate
	shared.Shared = true
	shared.Evidence = []Evidence{{Project: "/project-a", Kind: "创建操作记录"}, {Project: "/project-b", Kind: "命令引用"}}
	s.temps = []TempFile{shared}
	if s.validateTemp(p) == nil {
		t.Fatal("ordinary mode accepted shared directory")
	}
	if err := s.validateTempForPlan(p, true); err != nil {
		t.Fatalf("force should allow shared ownership: %v", err)
	}
	for _, scenario := range []string{"incomplete", "link", "changed", "outside"} {
		t.Run(scenario, func(t *testing.T) {
			v := candidate
			s.roots = []string{root}
			switch scenario {
			case "incomplete":
				v.Error = "fixture scan error"
			case "link":
				v.Link = true
			case "changed":
				v.Bytes++
			case "outside":
				s.roots = nil
			}
			s.temps = []TempFile{v}
			if s.validateTempForPlan(p, true) == nil {
				t.Fatal("force bypassed safety guard")
			}
		})
	}
}

func TestForceTempPlanRequiresExplicitConfirmationAndMovesToTrash(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("trash supported on macOS")
	}
	s := testService(t)
	root := t.TempDir()
	s.roots = []string{root}
	s.trashDir = t.TempDir()
	p := filepath.Join(root, "unknown.txt")
	if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	st := mustStat(t, p)
	s.temps = []TempFile{{Path: p, Name: "unknown.txt", Shared: true, Bytes: st.Size(), Mtime: st.ModTime().UnixNano()}}
	normal, err := s.PreviewClean("temp", []string{p})
	if err != nil || normal.Items[0].Blocked == "" {
		t.Fatalf("normal: %+v %v", normal, err)
	}
	plan, err := s.PreviewForceTempClean([]string{p})
	if err != nil || !plan.Force || plan.Items[0].Blocked != "" {
		t.Fatalf("force: %+v %v", plan, err)
	}
	if _, err = s.ExecuteClean(plan.Token, "确认清理"); err == nil {
		t.Fatal("force accepted ordinary confirmation")
	}
	if _, err = os.Stat(p); err != nil {
		t.Fatal("wrong confirmation changed file")
	}
	results, err := s.ExecuteClean(plan.Token, "确认强制清理")
	if err != nil || len(results) != 1 || !results[0].Success {
		t.Fatalf("execute: %+v %v", results, err)
	}
	if _, err = os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("source still exists")
	}
	entries, err := os.ReadDir(s.trashDir)
	if err != nil || len(entries) != 1 {
		t.Fatal("fixture not moved to isolated trash")
	}
}
