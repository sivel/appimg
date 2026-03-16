package resolver_test

import (
	"context"
	"testing"

	_ "github.com/sivel/appimg/internal/backend/github"
	_ "github.com/sivel/appimg/internal/backend/gitlab"
	"github.com/sivel/appimg/internal/resolver"
)

func TestErrNotFound_Error(t *testing.T) {
	err := &resolver.ErrNotFound{Name: "Bitwarden"}
	want := `package "Bitwarden" not found`
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestParse_GitHub(t *testing.T) {
	src, version, err := resolver.Parse(context.Background(), "github:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if version != "" {
		t.Errorf("version = %q, want empty", version)
	}
	if src.AppName != "repo" {
		t.Errorf("AppName = %q, want repo", src.AppName)
	}
	if src.Project != "owner/repo" {
		t.Errorf("Project = %q, want owner/repo", src.Project)
	}
	if src.Source != "github:owner/repo" {
		t.Errorf("Source = %q, want github:owner/repo", src.Source)
	}
}

func TestParse_GitHubPinnedVersion(t *testing.T) {
	src, version, err := resolver.Parse(context.Background(), "github:owner/repo@v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", version)
	}
	if src.Project != "owner/repo" {
		t.Errorf("Project = %q, want owner/repo", src.Project)
	}
}

func TestParse_GitLab(t *testing.T) {
	src, version, err := resolver.Parse(context.Background(), "gitlab:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if version != "" {
		t.Errorf("version = %q, want empty", version)
	}
	if src.Project != "owner/repo" {
		t.Errorf("Project = %q, want owner/repo", src.Project)
	}
}

func TestParse_GitHubNestedPath(t *testing.T) {
	// gitlab supports nested paths like namespace/group/repo
	src, _, err := resolver.Parse(context.Background(), "gitlab:namespace/group/repo")
	if err != nil {
		t.Fatal(err)
	}
	if src.Project != "namespace/group/repo" {
		t.Errorf("Project = %q, want namespace/group/repo", src.Project)
	}
}

func TestParse_InvalidGitHubSource(t *testing.T) {
	// github:repo with no owner is invalid
	_, _, err := resolver.Parse(context.Background(), "github:repo")
	if err == nil {
		t.Fatal("expected error for github:repo (no owner), got nil")
	}
}

func TestParse_CatalogPrefixStripped(t *testing.T) {
	// "catalog:github:owner/repo" should strip "catalog:" and then match github backend
	src, _, err := resolver.Parse(context.Background(), "catalog:github:owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if src.Project != "owner/repo" {
		t.Errorf("Project = %q, want owner/repo", src.Project)
	}
}
