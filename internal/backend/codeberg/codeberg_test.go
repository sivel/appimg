package codeberg_test

import (
	"testing"

	"github.com/sivel/appimg/internal/backend"
	_ "github.com/sivel/appimg/internal/backend/codeberg"
)

func TestCodebergRegistered(t *testing.T) {
	b, project, appName, ok := backend.Lookup("codeberg:owner/repo")
	if !ok {
		t.Fatal("codeberg backend not registered")
	}
	if project != "owner/repo" {
		t.Errorf("project = %q, want owner/repo", project)
	}
	if appName != "repo" {
		t.Errorf("appName = %q, want repo", appName)
	}
	if b == nil {
		t.Error("backend is nil")
	}
}
