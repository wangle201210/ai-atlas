package updates

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestPublicReleaseFallback(t *testing.T) {
	for _, scenario := range []string{"valid", "current", "missing-checksum", "invalid-checksum", "wrong-tag", "too-large", "non-rate-limit"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			payload := []byte("test artifact")
			digest := fmt.Sprintf("%x", sha256.Sum256(payload))
			name := "ai-atlas-darwin-arm64.zip"
			mux := http.NewServeMux()
			mux.HandleFunc("/repos/owner/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusForbidden)
				if scenario == "non-rate-limit" {
					fmt.Fprint(w, "access denied")
				} else {
					fmt.Fprint(w, "API rate limit exceeded")
				}
			})
			mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
				tag := "v0.1.5"
				if scenario == "wrong-tag" {
					tag = "v0.1.5-beta.1"
				}
				http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
			})
			mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
			mux.HandleFunc("/releases/download/v0.1.5/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "missing-checksum":
					http.NotFound(w, r)
				case "invalid-checksum":
					fmt.Fprintf(w, "invalid  %s\n", name)
				default:
					fmt.Fprintf(w, "%s  %s\n", digest, name)
				}
			})
			mux.HandleFunc("/releases/download/v0.1.5/"+name, func(w http.ResponseWriter, r *http.Request) {
				size := len(payload)
				if scenario == "too-large" {
					size = 501 << 20
				}
				w.Header().Set("Content-Length", fmt.Sprint(size))
				if r.Method != "HEAD" {
					w.Write(payload)
				}
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			upstream, err := github.New(github.Config{Repository: "owner/repo", BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			p := VerifiedProvider{&releaseFallback{Provider: upstream, client: server.Client(), releasesURL: server.URL + "/releases"}}
			req := updater.CheckRequest{CurrentVersion: "0.1.4", Platform: "darwin", Arch: "arm64"}
			if scenario == "current" {
				req.CurrentVersion = "0.1.5"
			}
			rel, err := p.Check(t.Context(), req)
			if scenario != "valid" && scenario != "current" {
				if err == nil {
					t.Fatal("invalid release accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "current" {
				if rel != nil {
					t.Fatal("current release offered")
				}
				return
			}
			if rel.Version != "0.1.5" || rel.Artifact.Filename != name || fmt.Sprintf("%x", rel.Verification.Digest) != digest {
				t.Fatalf("bad metadata: %+v", rel)
			}
			var downloaded strings.Builder
			if err = p.Download(t.Context(), rel, &downloaded, nil); err != nil || downloaded.String() != string(payload) {
				t.Fatalf("download failed: %v", err)
			}
			if _, err = p.Check(t.Context(), req); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("rate limited API retried %d times", calls)
			}
		})
	}
}
