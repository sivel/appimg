// Package backend defines the shared types, interface, and scoring logic
// used by all release hosting backends.
package backend

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sivel/appimg/internal/platform"
)

// Asset is a downloadable file attached to a release.
type Asset struct {
	Name               string
	BrowserDownloadURL string
	Size               int64
}

// Release holds release metadata and assets.
type Release struct {
	TagName     string
	PublishedAt string
	Prerelease  bool
	Draft       bool
	Assets      []Asset
}

// Options controls release resolution behaviour.
type Options struct {
	// Version pins to a specific release tag. Pre-releases are included when
	// a version is pinned regardless of Prerelease.
	Version string
	// Prerelease allows pre-releases to be considered when finding the latest release.
	Prerelease bool
	// AssetPattern overrides automatic asset selection with a glob pattern.
	AssetPattern string
	// VersionPrefix restricts release selection to tags beginning with this string,
	// useful for repos that publish multiple products (e.g. "desktop-" vs "cli-").
	VersionPrefix string
}

// ScoredAsset pairs an Asset with its selection score.
type ScoredAsset struct {
	Asset Asset
	Score int
}

// Backend is the interface implemented by each release hosting backend.
type Backend interface {
	LatestRelease(ctx context.Context, project string, opts Options) (*Release, error)
	AllReleases(ctx context.Context, project string, opts Options) ([]Release, error)
	ProjectURL(project string) string
	ReleaseURL(project, tag string) string
}

var registry = map[string]func() Backend{}

// Register registers a backend factory for the given source prefix (e.g. "github").
// It is typically called from an init() function in each backend package.
func Register(prefix string, factory func() Backend) {
	registry[prefix] = factory
}

// Lookup finds the backend, project path, and app name for a prefixed source string
// like "github:owner/repo" or "gitlab:ns/group/project".
// Returns (nil, "", "", false) if no registered backend matches the prefix.
func Lookup(source string) (Backend, string, string, bool) {
	for prefix, factory := range registry {
		full := prefix + ":"
		if strings.HasPrefix(source, full) {
			project := source[len(full):]
			parts := strings.Split(project, "/")
			appName := parts[len(parts)-1]
			return factory(), project, appName, true
		}
	}
	return nil, "", "", false
}

var penaltyKeywords = []string{"-lite", "-minimal", "-debug", "-dev"}

// SelectAsset picks the best AppImage asset for the current platform.
// If opts.AssetPattern is set it is used as a glob and the first match is returned.
func SelectAsset(assets []Asset, opts Options) (*Asset, error) {
	osInfo := platform.OS()
	return selectAsset(assets, opts, platform.Arch(), osInfo.ID, osInfo.Version)
}

func selectAsset(assets []Asset, opts Options, arch, distro, distroVersion string) (*Asset, error) {
	if opts.AssetPattern != "" {
		slog.Debug("selecting asset by pattern", "pattern", opts.AssetPattern)
		for i := range assets {
			matched, err := filepath.Match(opts.AssetPattern, assets[i].Name)
			if err != nil {
				return nil, fmt.Errorf("invalid asset pattern %q: %w", opts.AssetPattern, err)
			}
			slog.Debug("pattern match result", "asset", assets[i].Name, "matched", matched)
			if matched {
				return &assets[i], nil
			}
		}
		return nil, fmt.Errorf("no asset matching pattern %q", opts.AssetPattern)
	}

	ss, err := scoreAssets(assets, opts, arch, distro, distroVersion)
	if err != nil {
		return nil, err
	}
	return &ss[0].Asset, nil
}

// ScoreAssets returns all AppImage assets scored and sorted for the current platform.
func ScoreAssets(assets []Asset, opts Options) ([]ScoredAsset, error) {
	osInfo := platform.OS()
	return scoreAssets(assets, opts, platform.Arch(), osInfo.ID, osInfo.Version)
}

func scoreAssets(assets []Asset, opts Options, arch, distro, distroVersion string) ([]ScoredAsset, error) {
	var ss []ScoredAsset
	for _, a := range assets {
		if !strings.HasSuffix(strings.ToLower(a.Name), ".appimage") {
			continue
		}
		score := 0
		lower := strings.ToLower(a.Name)

		if strings.Contains(lower, strings.ToLower(arch)) {
			score += 10
		}
		if distro != "" && strings.Contains(lower, distro) {
			score += 5
			if distroVersion != "" {
				if av := distroVersionFromAsset(lower, distro); av != "" {
					score += versionProximityScore(av, distroVersion)
				}
			}
		}
		for _, kw := range penaltyKeywords {
			if strings.Contains(lower, kw) {
				score -= 5
			}
		}
		ss = append(ss, ScoredAsset{a, score})
	}

	if len(ss) == 0 {
		return nil, fmt.Errorf("no AppImage assets found in release")
	}

	slices.SortFunc(ss, func(a, b ScoredAsset) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return len(a.Asset.Name) - len(b.Asset.Name)
	})

	for _, s := range ss {
		slog.Debug("scored asset", "name", s.Asset.Name, "score", s.Score)
	}
	return ss, nil
}

func distroVersionFromAsset(assetLower, distro string) string {
	i := strings.Index(assetLower, distro)
	if i < 0 {
		return ""
	}
	rest := assetLower[i+len(distro):]
	for len(rest) > 0 && (rest[0] == '-' || rest[0] == '_') {
		rest = rest[1:]
	}
	if len(rest) > 0 && rest[0] == 'v' {
		rest = rest[1:]
	}
	k := 0
	for k < len(rest) && (rest[k] >= '0' && rest[k] <= '9' || rest[k] == '.') {
		k++
	}
	v := strings.TrimRight(rest[:k], ".")
	if len(v) == 0 || v[0] < '0' || v[0] > '9' || strings.Count(v, ".") > 1 {
		return ""
	}
	return v
}

func versionProximityScore(assetVer, sysVer string) int {
	a := versionToInt(assetVer)
	s := versionToInt(sysVer)
	switch {
	case a == s:
		return 5
	case a < s:
		proximity := 3 - (s-a)/100
		if proximity < 1 {
			return 1
		}
		return proximity
	default:
		return 1
	}
}

func versionToInt(v string) int {
	parts := strings.SplitN(v, ".", 2)
	major, _ := strconv.Atoi(parts[0])
	if len(parts) == 1 {
		return major * 100
	}
	minor, _ := strconv.Atoi(parts[1])
	return major*100 + minor
}

var rollingTags = map[string]struct{}{
	"nightly":    {},
	"latest":     {},
	"continuous": {},
	"rolling":    {},
	"edge":       {},
	"dev":        {},
	"unstable":   {},
	"preview":    {},
}

// IsRollingTag reports whether tag is a well-known rolling release tag name.
func IsRollingTag(tag string) bool {
	_, ok := rollingTags[strings.ToLower(tag)]
	return ok
}

// HasAppImageAssets reports whether any asset is an AppImage.
func HasAppImageAssets(assets []Asset) bool {
	return slices.ContainsFunc(assets, func(a Asset) bool {
		return strings.HasSuffix(strings.ToLower(a.Name), ".appimage")
	})
}
