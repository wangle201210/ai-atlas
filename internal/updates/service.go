package updates

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

type Engine interface {
	Check(context.Context) (*updater.Release, error)
	DownloadAndInstall(context.Context) error
	Restart(context.Context) error
}

type State struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Phase          string `json:"phase"`
	Notes          string `json:"notes"`
	ReleaseName    string `json:"releaseName"`
	ReleaseURL     string `json:"releaseURL"`
	CheckedAt      string `json:"checkedAt"`
	Written        int64  `json:"written"`
	Total          int64  `json:"total"`
	CanInstall     bool   `json:"canInstall"`
	Error          string `json:"error"`
}

type Service struct {
	mu            sync.Mutex
	engine        Engine
	state         State
	running       bool
	ctx           context.Context
	cancel        context.CancelFunc
	beforeRestart func() error
}

func New(version string, engine Engine, canInstall bool, beforeRestart func() error) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{engine: engine, state: State{CurrentVersion: version, Phase: "idle", ReleaseURL: ReleasesURL, CanInstall: canInstall}, ctx: ctx, cancel: cancel, beforeRestart: beforeRestart}
}
func Close(s *Service)           { s.cancel() }
func (s *Service) Status() State { s.mu.Lock(); defer s.mu.Unlock(); return s.state }

func (s *Service) Check() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("更新操作正在进行")
	}
	if s.state.Phase == "ready" {
		s.mu.Unlock()
		return errors.New("新版已下载，请重启安装")
	}
	s.running = true
	s.state = State{CurrentVersion: s.state.CurrentVersion, CanInstall: s.state.CanInstall, Phase: "checking", ReleaseURL: ReleasesURL}
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()
		rel, err := s.engine.Check(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = false
		s.state.CheckedAt = time.Now().Format(time.RFC3339)
		if err != nil {
			s.state.Phase = "error"
			s.state.Error = "检查失败，请检查网络后重试：" + err.Error()
			return
		}
		if rel == nil {
			s.state.Phase = "current"
			return
		}
		s.state.Phase = "available"
		s.state.LatestVersion = rel.Version
		s.state.ReleaseName = rel.Name
		s.state.Notes = rel.Notes
		s.state.Total = rel.Artifact.Size
	}()
	return nil
}
func (s *Service) Download() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("更新操作正在进行")
	}
	if !s.state.CanInstall {
		s.mu.Unlock()
		return errors.New("请在安装好的 macOS 应用中更新；浏览器和开发模式不支持替换应用")
	}
	if s.state.Phase != "available" {
		s.mu.Unlock()
		return errors.New("请先检查可用的新版本")
	}
	s.running = true
	s.state.Phase = "downloading"
	s.state.Error = ""
	s.state.Written = 0
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, 15*time.Minute)
		defer cancel()
		err := s.engine.DownloadAndInstall(ctx)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = false
		if err != nil {
			s.state.Phase = "error"
			s.state.Error = "更新包下载或校验失败，可重新检查后重试：" + err.Error()
			return
		}
		s.state.Phase = "ready"
	}()
	return nil
}
func (s *Service) Restart() error {
	s.mu.Lock()
	if s.running || s.state.Phase != "ready" || !s.state.CanInstall {
		s.mu.Unlock()
		return errors.New("更新尚未准备完成")
	}
	s.running = true
	s.state.Phase = "restarting"
	s.mu.Unlock()
	var err error
	if s.beforeRestart != nil {
		err = s.beforeRestart()
	}
	if err == nil {
		err = s.engine.Restart(s.ctx)
	}
	if err != nil {
		s.mu.Lock()
		s.running = false
		s.state.Phase = "ready"
		s.state.Error = fmt.Sprintf("无法重启安装：%v", err)
		s.mu.Unlock()
	}
	return err
}

// ReportProgress receives trusted events from Wails, not frontend-provided data.
func ReportProgress(s *Service, written, total int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running && s.state.Phase == "downloading" {
		s.state.Written = written
		if total > 0 {
			s.state.Total = total
		}
	}
}
func ReportPhase(s *Service, phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running && (phase == "verifying" || phase == "installing") {
		s.state.Phase = phase
	}
}

// Attach wires the engine before app.Run starts accepting frontend calls.
func Attach(s *Service, engine Engine) { s.engine = engine }
