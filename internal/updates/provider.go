package updates

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const Repository = "wangle201210/ai-atlas"
const ReleasesURL = "https://github.com/" + Repository + "/releases"

// VerifiedProvider requires the release checksum; the upstream GitHub provider
// otherwise permits a missing sidecar. No account credentials are sent.
type VerifiedProvider struct{ updater.Provider }

func (p VerifiedProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	rel, err := p.Provider.Check(ctx, req)
	if err != nil || rel == nil {
		return rel, err
	}
	if rel.Verification == nil || rel.Verification.DigestAlgo != "sha256" || len(rel.Verification.Digest) != sha256.Size {
		return nil, errors.New("发布版本缺少有效的 SHA256SUMS 校验值，暂不能自动更新")
	}
	if rel.Artifact.Size <= 0 || rel.Artifact.Size > 500<<20 {
		return nil, errors.New("更新包大小无效或超过 500 MiB")
	}
	return rel, nil
}
func NewProvider() (updater.Provider, error) {
	client := &http.Client{Timeout: 15 * time.Minute}
	provider, err := github.New(github.Config{Repository: Repository, ChecksumAsset: "SHA256SUMS", HTTPClient: client, AssetMatcher: matchAsset})
	if err != nil {
		return nil, err
	}
	return VerifiedProvider{&releaseFallback{Provider: provider, client: client, releasesURL: ReleasesURL}}, nil
}
func matchAsset(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	// This release pipeline distributes complete macOS app bundles, never loose binaries.
	if req.Platform != "darwin" || (req.Arch != "arm64" && req.Arch != "amd64") {
		return -1
	}
	want := fmt.Sprintf("ai-atlas-%s-%s.zip", req.Platform, req.Arch)
	for i, a := range assets {
		if a.Name == want {
			return i
		}
	}
	return -1
}
