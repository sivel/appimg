package cache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adrg/xdg"
)

func overrideXDGCacheHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	xdg.Reload()
	t.Cleanup(xdg.Reload)
}

func setFeedURL(t *testing.T, url string) {
	t.Helper()
	orig := feedURL
	feedURL = url
	t.Cleanup(func() { feedURL = orig })
}

func TestIsExpired_Missing(t *testing.T) {
	overrideXDGCacheHome(t)
	expired, err := IsExpired()
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Error("expected expired=true when cache file is missing")
	}
}

func TestIsExpired_Fresh(t *testing.T) {
	overrideXDGCacheHome(t)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	expired, err := IsExpired()
	if err != nil {
		t.Fatal(err)
	}
	if expired {
		t.Error("expected expired=false for a just-written cache file")
	}
}

func TestIsExpired_Old(t *testing.T) {
	overrideXDGCacheHome(t)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-(cacheTTL + time.Hour))
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}

	expired, err := IsExpired()
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Error("expected expired=true for old cache file")
	}
}

func TestRefresh_OK(t *testing.T) {
	overrideXDGCacheHome(t)

	body := `{"items":["hello"]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	setFeedURL(t, srv.URL)

	if err := Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	path, _ := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Errorf("cache content = %q, want %q", data, body)
	}
}

func TestRefresh_NonOK(t *testing.T) {
	overrideXDGCacheHome(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	setFeedURL(t, srv.URL)

	if err := Refresh(context.Background()); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestRefreshIfExpired_SkipsWhenFresh(t *testing.T) {
	overrideXDGCacheHome(t)

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	setFeedURL(t, srv.URL)

	if err := RefreshIfExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("expected server not to be called for a fresh cache")
	}
}

func TestRefreshIfExpired_RefreshesWhenMissing(t *testing.T) {
	overrideXDGCacheHome(t)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	setFeedURL(t, srv.URL)

	if err := RefreshIfExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("expected server to be called for missing cache")
	}
}
