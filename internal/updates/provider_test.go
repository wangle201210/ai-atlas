package updates

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

type testHost struct{}

func (testHost) Emit(string, ...any) bool         { return true }
func (testHost) OnEvent(string, func(any)) func() { return func() {} }
func (testHost) OpenWindow(updater.WindowOptions) updater.WindowHandle {
	panic("tests must not create windows")
}
func (testHost) Quit() { panic("tests must not replace or quit a real application") }
func testZip(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "ai-atlas.app/Contents/MacOS/ai-atlas"}
	h.SetMode(0755)
	f, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("isolated test artifact")); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestReleaseSelectionAndVerifiedStaging(t *testing.T) {
	for _, scenario := range []string{"valid", "missing-checksum", "bad-checksum", "no-release", "older", "wrong-arch", "rate-limit"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			archive := testZip(t)
			sum := sha256.Sum256(archive)
			digest := hex.EncodeToString(sum[:])
			if scenario == "bad-checksum" {
				digest = fmt.Sprintf("%064d", 0)
			}
			mux := http.NewServeMux()
			server := httptest.NewServer(mux)
			defer server.Close()
			name := "ai-atlas-darwin-arm64.zip"
			if scenario == "wrong-arch" {
				name = "ai-atlas-darwin-amd64.zip"
			}
			mux.HandleFunc("GET /repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
				if scenario == "no-release" {
					w.WriteHeader(404)
					return
				}
				if scenario == "rate-limit" {
					w.WriteHeader(403)
					return
				}
				tag := "v0.2.0"
				if scenario == "older" {
					tag = "v0.1.0"
				}
				assets := []map[string]any{{"name": name, "size": len(archive), "browser_download_url": server.URL + "/artifact"}}
				if scenario != "missing-checksum" {
					assets = append(assets, map[string]any{"name": "SHA256SUMS", "browser_download_url": server.URL + "/checksums"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "name": "AI Atlas", "body": "Release notes", "assets": assets})
			})
			mux.HandleFunc("GET /artifact", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) })
			mux.HandleFunc("GET /checksums", func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprintf(w, "%s  %s\n", digest, name) })
			gh, err := github.New(github.Config{Repository: "owner/repo", BaseURL: server.URL, ChecksumAsset: "SHA256SUMS", AssetMatcher: matchAsset})
			if err != nil {
				t.Fatal(err)
			}
			u := updater.New(testHost{})
			if err = u.Init(updater.Config{CurrentVersion: "0.1.1", Platform: "darwin", Arch: "arm64", Providers: []updater.Provider{VerifiedProvider{gh}}, Window: updater.WindowNone}); err != nil {
				t.Fatal(err)
			}
			rel, err := u.Check(t.Context())
			switch scenario {
			case "missing-checksum", "wrong-arch", "rate-limit":
				if err == nil {
					t.Fatal("invalid release accepted")
				}
				return
			case "no-release", "older":
				if err != nil || rel != nil {
					t.Fatalf("unexpected update: %+v %v", rel, err)
				}
				return
			}
			if err != nil || rel == nil {
				t.Fatalf("check failed: %v", err)
			}
			err = u.DownloadAndInstall(context.Background())
			if scenario == "bad-checksum" {
				if err == nil || u.DownloadedPath() != "" {
					t.Fatal("corrupt download staged")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(u.DownloadedPath(), "Contents/MacOS/ai-atlas")
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "isolated test artifact" {
				t.Fatalf("bundle not extracted: %s %v", target, err)
			}
		})
	}
}
