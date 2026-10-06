package atlas

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDiagnostics(t *testing.T) {
	s := testService(t)
	if _, err := s.ExportDiagnostics(); err == nil {
		t.Fatal("missing save dialog should report an error")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "diagnostics.json")
	AttachDiagnosticPicker(s, func() (string, error) { return path, nil })
	saved, err := s.ExportDiagnostics()
	if err != nil || saved != path {
		t.Fatalf("export: %q, %v", saved, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(data) || strings.Contains(string(data), s.home) || strings.Contains(string(data), s.dbPath) {
		t.Fatal("expected valid redacted JSON")
	}
	AttachDiagnosticPicker(s, func() (string, error) { return "", nil })
	if saved, err = s.ExportDiagnostics(); err != nil || saved != "" {
		t.Fatalf("cancel: %q, %v", saved, err)
	}
	wantErr := errors.New("dialog failed")
	AttachDiagnosticPicker(s, func() (string, error) { return "", wantErr })
	if _, err = s.ExportDiagnostics(); !errors.Is(err, wantErr) {
		t.Fatalf("dialog error lost: %v", err)
	}
	// A failed replacement must leave the destination intact and remove staging files.
	target := filepath.Join(dir, "directory.json")
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	AttachDiagnosticPicker(s, func() (string, error) { return target, nil })
	if _, err = s.ExportDiagnostics(); err == nil {
		t.Fatal("write failure should report an error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected staging files: %v", entries)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("existing diagnostic was changed")
	}
}
