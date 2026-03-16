// Package gitlab implements the GitLab releases backend for appimg.
package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/httpclient"
)

var errNotFound = errors.New("not found")

var apiBase = "https://gitlab.com/api/v4"

// GitLab implements backend.Backend for GitLab releases.
type GitLab struct{}

// New returns a GitLab backend instance.
func New() *GitLab { return &GitLab{} }

func (g *GitLab) ProjectURL(project string) string {
	return "https://gitlab.com/" + project
}

func (g *GitLab) ReleaseURL(project, tag string) string {
	return "https://gitlab.com/" + project + "/-/releases/" + tag
}

// encodePath URL-encodes a GitLab project path (replaces / with %2F).
func encodePath(project string) string {
	return strings.ReplaceAll(project, "/", "%2F")
}

// LatestRelease returns the resolved release for the given project path according to opts.
func (g *GitLab) LatestRelease(ctx context.Context, project string, opts backend.Options) (*backend.Release, error) {
	if opts.Version != "" {
		slog.Debug("fetching release by version", "project", project, "version", opts.Version)
		return releaseByVersion(ctx, project, opts.Version)
	}
	return latestFiltered(ctx, project, opts)
}

// AllReleases returns all non-upcoming releases for the given project according to opts.
// Results are paginated automatically.
func (g *GitLab) AllReleases(ctx context.Context, project string, opts backend.Options) ([]backend.Release, error) {
	var all []backend.Release
	page := 1
	for {
		url := fmt.Sprintf("%s/projects/%s/releases?per_page=100&page=%d", apiBase, encodePath(project), page)
		var batch []glRelease
		if err := apiGet(ctx, url, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, r := range batch {
			if r.UpcomingRelease {
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

// glRelease is the internal JSON struct for decoding GitLab API responses.
type glRelease struct {
	TagName         string   `json:"tag_name"`
	ReleasedAt      string   `json:"released_at"`
	UpcomingRelease bool     `json:"upcoming_release"`
	Assets          glAssets `json:"assets"`
}

type glAssets struct {
	Links []glLink `json:"links"`
}

type glLink struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	LinkType string `json:"link_type"`
}

func toRelease(r glRelease) backend.Release {
	assets := make([]backend.Asset, 0, len(r.Assets.Links))
	for _, l := range r.Assets.Links {
		assets = append(assets, backend.Asset{
			Name:               l.Name,
			BrowserDownloadURL: l.URL,
			Size:               0, // not available from GitLab releases API
		})
	}
	return backend.Release{
		TagName:     r.TagName,
		PublishedAt: r.ReleasedAt,
		Prerelease:  false, // GitLab has no prerelease concept
		Draft:       r.UpcomingRelease,
		Assets:      assets,
	}
}

func latestFiltered(ctx context.Context, project string, opts backend.Options) (*backend.Release, error) {
	url := fmt.Sprintf("%s/projects/%s/releases?per_page=100", apiBase, encodePath(project))
	var releases []glRelease
	if err := apiGet(ctx, url, &releases); err != nil {
		return nil, err
	}
	for i := range releases {
		r := &releases[i]
		if r.UpcomingRelease {
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
	return nil, fmt.Errorf("no matching AppImage release found for %s", project)
}

func releaseByVersion(ctx context.Context, project, version string) (*backend.Release, error) {
	// Try exact tag, then with/without v prefix.
	candidates := []string{version}
	if strings.HasPrefix(version, "v") {
		candidates = append(candidates, strings.TrimPrefix(version, "v"))
	} else {
		candidates = append(candidates, "v"+version)
	}

	for _, tag := range candidates {
		slog.Debug("trying release tag", "tag", tag)
		url := fmt.Sprintf("%s/projects/%s/releases/%s", apiBase, encodePath(project), tag)
		var rel glRelease
		err := apiGet(ctx, url, &rel)
		if err != nil {
			if errors.Is(err, errNotFound) {
				continue
			}
			return nil, err
		}
		if rel.UpcomingRelease {
			continue
		}
		r := toRelease(rel)
		return &r, nil
	}
	return nil, fmt.Errorf("release %q not found for %s", version, project)
}

func init() {
	backend.Register("gitlab", func() backend.Backend { return New() })
}

func apiGet(ctx context.Context, url string, out any) error {
	slog.Debug("gitlab api request", "url", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "appimg")
	if tok := os.Getenv("GITLAB_TOKEN"); tok != "" {
		req.Header.Set("PRIVATE-TOKEN", tok)
	}

	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab api: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("gitlab api rate limit exceeded; set GITLAB_TOKEN to increase the limit")
	}
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gitlab api: unexpected status %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
