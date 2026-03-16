// Package resolver parses appimg source strings into backend+project pairs.
package resolver

import (
	"context"
	"fmt"
	"strings"

	"github.com/sivel/appimg/internal/backend"
	"github.com/sivel/appimg/internal/cache"
	"github.com/sivel/appimg/internal/catalog"
)

// ErrNotFound is returned when a requested package cannot be found.
type ErrNotFound struct {
	Name string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("package %q not found", e.Name)
}

// ResolvedSource holds the information needed to download an AppImage.
type ResolvedSource struct {
	// AppName is the canonical name used for the database key, file naming, etc.
	AppName string
	// Source is the source string stored in the database (e.g. "catalog:Bitwarden").
	Source      string
	Backend     backend.Backend
	Project     string
	Categories  []string
	Description string
}

// Parse parses a source argument of the form:
//
//	Bitwarden                          -> catalog lookup
//	Bitwarden@2024.1.0                 -> catalog lookup, pinned version
//	github:owner/repo                  -> direct github backend
//	github:owner/repo@v1.2.3          -> direct github backend, pinned version
//	gitlab:namespace/project           -> direct gitlab backend
//	gitlab:namespace/project@v1.2.3   -> direct gitlab backend, pinned version
//
// It returns the resolved source and the version string (empty if not pinned).
func Parse(ctx context.Context, arg string) (*ResolvedSource, string, error) {
	version := ""

	if idx := strings.LastIndex(arg, "@"); idx != -1 {
		version = arg[idx+1:]
		arg = arg[:idx]
	}

	arg = strings.TrimPrefix(arg, "catalog:")

	if b, project, appName, ok := backend.Lookup(arg); ok {
		parts := strings.Split(project, "/")
		if len(parts) < 2 || parts[0] == "" || appName == "" {
			prefix, _, _ := strings.Cut(arg, ":")
			return nil, "", fmt.Errorf("invalid %s source %q: expected %s:owner/repo", prefix, arg, prefix)
		}
		return &ResolvedSource{
			AppName: appName,
			Source:  arg,
			Backend: b,
			Project: project,
		}, version, nil
	}

	if err := cache.RefreshIfExpired(ctx); err != nil {
		return nil, "", fmt.Errorf("refresh catalog: %w", err)
	}
	feed, err := catalog.Load()
	if err != nil {
		return nil, "", err
	}
	entry, ok := catalog.Find(feed, arg)
	if !ok {
		// Cache may be stale; force refresh and retry once.
		if err := cache.Refresh(ctx); err != nil {
			return nil, "", err
		}
		feed, err = catalog.Load()
		if err != nil {
			return nil, "", err
		}
		entry, ok = catalog.Find(feed, arg)
		if !ok {
			return nil, "", &ErrNotFound{Name: arg}
		}
	}
	repo, ok := entry.GitHubRepo()
	if !ok {
		return nil, "", fmt.Errorf("catalog entry %q has no GitHub link", arg)
	}
	ghBackend, _, _, ok := backend.Lookup("github:" + repo)
	if !ok {
		return nil, "", fmt.Errorf("github backend not registered")
	}
	return &ResolvedSource{
		AppName:     entry.Name,
		Source:      "catalog:" + entry.Name,
		Backend:     ghBackend,
		Project:     repo,
		Categories:  entry.Categories,
		Description: entry.Description,
	}, version, nil
}
