package updates

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

type fakeEngine struct {
	gate                              chan struct{}
	release                           *updater.Release
	checkErr, downloadErr, restartErr error
	restarted                         bool
}

func (f *fakeEngine) Check(ctx context.Context) (*updater.Release, error) {
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.release, f.checkErr
}
func (f *fakeEngine) DownloadAndInstall(context.Context) error { return f.downloadErr }
func (f *fakeEngine) Restart(context.Context) error            { f.restarted = true; return f.restartErr }
func awaitPhase(t *testing.T, s *Service, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	for {
		if s.Status().Phase == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wanted %s, got %+v", want, s.Status())
		case <-time.After(time.Millisecond):
		}
	}
}
func TestExplicitDownloadAndRestart(t *testing.T) {
	f := &fakeEngine{release: &updater.Release{Version: "0.2.0", Notes: "new version", Artifact: updater.Artifact{Size: 100}}}
	s := New("0.1.1", f, true, nil)
	defer Close(s)
	if err := s.Download(); err == nil {
		t.Fatal("download allowed before check")
	}
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	awaitPhase(t, s, "available")
	if s.Status().LatestVersion != "0.2.0" || f.restarted {
		t.Fatal("check triggered install")
	}
	if err := s.Restart(); err == nil {
		t.Fatal("restart allowed before download")
	}
	if err := s.Download(); err != nil {
		t.Fatal(err)
	}
	awaitPhase(t, s, "ready")
	if f.restarted {
		t.Fatal("download auto-restarted app")
	}
	if err := s.Restart(); err != nil {
		t.Fatal(err)
	}
	if !f.restarted {
		t.Fatal("restart not delegated")
	}
}
func TestConcurrentCheckAndServerInstallBlocked(t *testing.T) {
	f := &fakeEngine{gate: make(chan struct{}), release: &updater.Release{Version: "0.2.0"}}
	s := New("0.1.1", f, false, nil)
	defer Close(s)
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(); err == nil {
		t.Fatal("concurrent check allowed")
	}
	close(f.gate)
	awaitPhase(t, s, "available")
	if err := s.Download(); err == nil {
		t.Fatal("browser installation allowed")
	}
}
func TestErrorAndRestartGuard(t *testing.T) {
	f := &fakeEngine{release: &updater.Release{Version: "0.2.0"}, downloadErr: errors.New("checksum mismatch")}
	s := New("0.1.1", f, true, func() error { return errors.New("scan active") })
	defer Close(s)
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
	awaitPhase(t, s, "available")
	if err := s.Download(); err != nil {
		t.Fatal(err)
	}
	awaitPhase(t, s, "error")
	if err := s.Restart(); err == nil {
		t.Fatal("restart after failed verification")
	}
	s.mu.Lock()
	s.state.Phase = "ready"
	s.mu.Unlock()
	if err := s.Restart(); err == nil || f.restarted {
		t.Fatal("restart ignored busy application")
	}
	if s.Status().Phase != "ready" {
		t.Fatal("cannot retry restart")
	}
}
func TestNoReleaseAndNetworkError(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "no release", true: "network error"}[fail], func(t *testing.T) {
			f := &fakeEngine{}
			if fail {
				f.checkErr = errors.New("network offline")
			}
			s := New("0.1.1", f, true, nil)
			defer Close(s)
			if err := s.Check(); err != nil {
				t.Fatal(err)
			}
			want := "current"
			if fail {
				want = "error"
			}
			awaitPhase(t, s, want)
		})
	}
}
