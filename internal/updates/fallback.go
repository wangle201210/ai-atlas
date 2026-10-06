package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"golang.org/x/mod/semver"
)

// Public release downloads remain available when the anonymous REST quota is exhausted.
// Download is delegated to the original provider; VerifiedProvider still validates all metadata.
type releaseFallback struct {
	updater.Provider
	client       *http.Client
	releasesURL  string
	mu           sync.Mutex
	limitedUntil time.Time
}

func isRateLimit(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "rate limit") || strings.Contains(text, "api 429")
}
func (p *releaseFallback) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	p.mu.Lock()
	limited := time.Now().Before(p.limitedUntil)
	p.mu.Unlock()
	if !limited {
		rel, err := p.Provider.Check(ctx, req)
		if !isRateLimit(err) {
			return rel, err
		}
		p.mu.Lock()
		p.limitedUntil = time.Now().Add(time.Hour)
		p.mu.Unlock()
	}
	rel, err := p.checkPublic(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API 请求额度已用尽，公开 Release 备用查询也未成功；请稍后重试或从 Releases 页面手动下载：%w", err)
	}
	return rel, nil
}
func (p *releaseFallback) request(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AI-Atlas-Updater")
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("公开 Release 返回 HTTP %d", res.StatusCode)
	}
	return res, nil
}
func (p *releaseFallback) checkPublic(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	res, err := p.request(ctx, http.MethodHead, p.releasesURL+"/latest")
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	prefix := p.releasesURL + "/tag/"
	finalURL := res.Request.URL.String()
	if !strings.HasPrefix(finalURL, prefix) {
		return nil, errors.New("无法确定本仓库最新正式版本")
	}
	tag := strings.TrimPrefix(finalURL, prefix)
	if !semver.IsValid(tag) || semver.Canonical(tag) != tag || semver.Prerelease(tag) != "" {
		return nil, errors.New("发布标签不是有效的正式版本")
	}
	current := "v" + strings.TrimPrefix(req.CurrentVersion, "v")
	if !semver.IsValid(current) {
		return nil, errors.New("当前版本号无效")
	}
	if semver.Compare(tag, current) <= 0 {
		return nil, nil
	}
	if req.Platform != "darwin" || (req.Arch != "arm64" && req.Arch != "amd64") {
		return nil, errors.New("此平台暂无自动更新安装包")
	}
	name := fmt.Sprintf("ai-atlas-%s-%s.zip", req.Platform, req.Arch)
	base := p.releasesURL + "/download/" + tag + "/"
	checksum, err := p.request(ctx, http.MethodGet, base+"SHA256SUMS")
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(checksum.Body, 64*1024+1))
	checksum.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(raw) > 64*1024 {
		return nil, errors.New("校验文件过大")
	}
	var digest []byte
	for line := range strings.SplitSeq(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if digest != nil {
			return nil, errors.New("校验文件包含重复条目")
		}
		digest, err = hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return nil, errors.New("更新包 SHA-256 校验值无效")
		}
	}
	if digest == nil {
		return nil, errors.New("发布版本缺少对应安装包的 SHA-256 校验值")
	}
	artifact, err := p.request(ctx, http.MethodHead, base+name)
	if err != nil {
		return nil, err
	}
	artifact.Body.Close()
	if artifact.ContentLength <= 0 || artifact.ContentLength > 500<<20 {
		return nil, errors.New("更新包大小无效或超过 500 MiB")
	}
	return &updater.Release{
		Version: strings.TrimPrefix(tag, "v"), Channel: "stable", Name: "AI Atlas " + tag,
		Notes:        "通过公开 Release 查询到新版本。完整更新说明请点击“查看 GitHub Releases”。",
		Artifact:     updater.Artifact{Filename: name, Filetype: "zip", Size: artifact.ContentLength, Platform: req.Platform, Arch: req.Arch},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: digest},
		Metadata:     map[string]any{"github.asset.url": base + name, "github.release.tag": tag, "github.release.htmlURL": finalURL},
	}, nil
}
