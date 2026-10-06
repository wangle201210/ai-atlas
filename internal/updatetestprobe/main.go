// Command updatetestprobe is an isolated updater integration fixture, never shipped.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

var version = "1.0.0"

type host struct{}

func (host) Emit(string, ...any) bool                              { return true }
func (host) OnEvent(string, func(any)) func()                      { return func() {} }
func (host) OpenWindow(updater.WindowOptions) updater.WindowHandle { panic("no UI in fixture") }
func (host) Quit()                                                 { os.Exit(0) }

type provider struct {
	path string
	data []byte
}

func (provider) Name() string { return "isolated-fixture" }
func (p provider) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	sum := sha256.Sum256(p.data)
	return &updater.Release{Version: "2.0.0", Artifact: updater.Artifact{Filename: "fixture.zip", Size: int64(len(p.data))}, Verification: &updater.Verification{DigestAlgo: "sha256", Digest: sum[:]}}, nil
}
func (p provider) Download(_ context.Context, _ *updater.Release, w io.Writer, progress func(int64, int64)) error {
	f, err := os.Open(p.path)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(w, f)
	progress(n, int64(len(p.data)))
	return err
}
func main() {
	updater.HandleHelperMode()
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(exe))))
	if !strings.Contains(root, "/ai-atlas-update-smoke-") {
		panic("fixture refuses to run outside its isolated directory")
	}
	db := filepath.Join(root, "ledger.sqlite")
	data, err := os.ReadFile(db)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(data)
	if version == "2.0.0" {
		raw, _ := json.Marshal(map[string]string{"version": version, "ledgerSHA256": hex.EncodeToString(sum[:])})
		if err = os.WriteFile(filepath.Join(root, "result.json"), raw, 0600); err != nil {
			panic(err)
		}
		return
	}
	archive := filepath.Join(root, "update.zip")
	data, err = os.ReadFile(archive)
	if err != nil {
		panic(err)
	}
	u := updater.New(host{})
	if err = u.Init(updater.Config{CurrentVersion: version, Providers: []updater.Provider{provider{archive, data}}, Window: updater.WindowNone}); err != nil {
		panic(err)
	}
	if _, err = u.Check(context.Background()); err != nil {
		panic(err)
	}
	if err = u.DownloadAndInstall(context.Background()); err != nil {
		panic(err)
	}
	if err = u.Restart(context.Background()); err != nil {
		panic(err)
	}
	fmt.Println("restart requested")
}
