// Package github implements a GitHub-compatible releases backend for appimg.
// It works with GitHub and any compatible API (e.g. Forgejo/Gitea instances
// like Codeberg) via NewWithConfig.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/httpclient"
)

const (
	userAgent = "appimg"
	apiVer    = "2022-11-28"
)

// Config holds the per-instance configuration for a GitHub-compatible backend.
type Config struct {
	// APIBase is the API root URL, e.g. "https://api.github.com" or
	// "https://codeberg.org/api/v1".
	APIBase string
	// HostURL is the web host URL used for building project and release links,
	// e.g. "https://github.com" or "https://codeberg.org".
	HostURL string
	// TokenEnvVar is the environment variable name for the auth token,
	// e.g. "GITHUB_TOKEN" or "CODEBERG_TOKEN".
	TokenEnvVar string
	// ExtraHeaders are additional HTTP headers sent with every API request.
	// GitHub requires Accept and X-GitHub-Api-Version; other hosts ignore them.
	ExtraHeaders map[string]string
}

// GitHub implements backend.Backend for GitHub and GitHub-compatible APIs.
type GitHub struct {
	cfg Config
}

// New returns a GitHub backend instance with GitHub.com defaults.
func New() *GitHub {
	return NewWithConfig(Config{
		APIBase:     "https://api.github.com",
		HostURL:     "https://github.com",
		TokenEnvVar: "GITHUB_TOKEN",
		ExtraHeaders: map[string]string{
			"Accept":               "application/vnd.github+json",
			"X-GitHub-Api-Version": apiVer,
		},
	})
}

// NewWithConfig returns a GitHub-compatible backend for the given config.
// Use this to target Forgejo/Gitea instances such as Codeberg.
func NewWithConfig(cfg Config) *GitHub {
	return &GitHub{cfg: cfg}
}

func (g *GitHub) ProjectURL(project string) string {
	return g.cfg.HostURL + "/" + project
}

func (g *GitHub) ReleaseURL(project, tag string) string {
	return g.cfg.HostURL + "/" + project + "/releases/tag/" + tag
}

// LatestRelease returns the resolved release for the given owner/repo according to opts.
// Releases that contain no AppImage assets are automatically skipped, so repos that
// publish multiple products under different tag prefixes (e.g. "desktop-" vs "cli-")
// resolve correctly without configuration. Use VersionPrefix to disambiguate further
// when multiple products in the same repo both ship AppImages.
func (g *GitHub) LatestRelease(ctx context.Context, repo string, opts backend.Options) (*backend.Release, error) {
	if opts.Version != "" {
		slog.Debug("fetching release by version", "repo", repo, "version", opts.Version)
		return g.releaseByVersion(ctx, repo, opts.Version)
	}
	// Fast path: no prefix filter and stable only — try /releases/latest first.
	if opts.VersionPrefix == "" && !opts.Prerelease {
		slog.Debug("trying /releases/latest", "repo", repo)
		rel, err := g.latestStable(ctx, repo)
		if err != nil {
			return nil, err
		}
		if backend.HasAppImageAssets(rel.Assets) {
			slog.Debug("using latest release", "repo", repo, "tag", rel.TagName)
			return rel, nil
		}
		slog.Debug("latest release has no AppImage assets, searching release list", "repo", repo, "tag", rel.TagName)
	}
	return g.latestFiltered(ctx, repo, opts)
}

// AllReleases returns all non-draft releases for the given repo according to opts.
// Results are paginated automatically.
func (g *GitHub) AllReleases(ctx context.Context, repo string, opts backend.Options) ([]backend.Release, error) {
	var all []backend.Release
	page := 1
	for {
		url := fmt.Sprintf("%s/repos/%s/releases?per_page=100&page=%d", g.cfg.APIBase, repo, page)
		var batch []ghRelease
		if err := g.apiGet(ctx, url, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, r := range batch {
			if r.Draft {
				continue
			}
			if !opts.Prerelease && r.Prerelease {
				continue
			}
			if opts.VersionPrefix != "" && !strings.HasPrefix(r.TagName, opts.VersionPrefix) {
				continue
			}
			all = append(all, toRelease(r))
		}
		page++
	}
	return all, nil
}

// ghRelease is the internal JSON struct for decoding GitHub-compatible API responses.
type ghRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt string    `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

func toRelease(r ghRelease) backend.Release {
	assets := make([]backend.Asset, len(r.Assets))
	for i, a := range r.Assets {
		assets[i] = backend.Asset{Name: a.Name, BrowserDownloadURL: a.BrowserDownloadURL, Size: a.Size}
	}
	return backend.Release{TagName: r.TagName, PublishedAt: r.PublishedAt, Prerelease: r.Prerelease, Draft: r.Draft, Assets: assets}
}

func (g *GitHub) latestStable(ctx context.Context, repo string) (*backend.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", g.cfg.APIBase, repo)
	var rel ghRelease
	if err := g.apiGet(ctx, url, &rel); err != nil {
		return nil, err
	}
	r := toRelease(rel)
	return &r, nil
}

func (g *GitHub) latestFiltered(ctx context.Context, repo string, opts backend.Options) (*backend.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=100", g.cfg.APIBase, repo)
	var releases []ghRelease
	if err := g.apiGet(ctx, url, &releases); err != nil {
		return nil, err
	}
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if !opts.Prerelease && r.Prerelease {
			continue
		}
		if opts.VersionPrefix != "" && !strings.HasPrefix(r.TagName, opts.VersionPrefix) {
			continue
		}
		rel := toRelease(*r)
		if !backend.HasAppImageAssets(rel.Assets) {
			slog.Debug("skipping release (no AppImage assets)", "tag", r.TagName)
			continue
		}
		slog.Debug("selected release", "tag", r.TagName)
		return &rel, nil
	}
	return nil, fmt.Errorf("no matching AppImage release found for %s", repo)
}

func (g *GitHub) releaseByVersion(ctx context.Context, repo, version string) (*backend.Release, error) {
	candidates := []string{version}
	if strings.HasPrefix(version, "v") {
		candidates = append(candidates, strings.TrimPrefix(version, "v"))
	} else {
		candidates = append(candidates, "v"+version)
	}

	url := fmt.Sprintf("%s/repos/%s/releases?per_page=100", g.cfg.APIBase, repo)
	var releases []ghRelease
	if err := g.apiGet(ctx, url, &releases); err != nil {
		return nil, err
	}
	for _, tag := range candidates {
		slog.Debug("trying release tag", "tag", tag)
		for i := range releases {
			if releases[i].TagName == tag && !releases[i].Draft {
				slog.Debug("found release", "tag", releases[i].TagName)
				r := toRelease(releases[i])
				return &r, nil
			}
		}
	}
	return nil, fmt.Errorf("release %q not found for %s", version, repo)
}

func (g *GitHub) apiGet(ctx context.Context, url string, out any) error {
	slog.Debug("api request", "host", g.cfg.HostURL, "url", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range g.cfg.ExtraHeaders {
		req.Header.Set(k, v)
	}
	if tok := os.Getenv(g.cfg.TokenEnvVar); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return fmt.Errorf("api: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("api rate limit exceeded; set %s to increase the limit", g.cfg.TokenEnvVar)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("repo or release not found: %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api: unexpected status %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func init() {
	backend.Register("github", func() backend.Backend { return New() })
}
