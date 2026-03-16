package transfer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/schollz/progressbar/v3"
)

func silentBar() *progressbar.ProgressBar {
	return progressbar.NewOptions64(-1, progressbar.OptionSetWriter(io.Discard))
}

func TestDownload_OK(t *testing.T) {
	body := "hello appimage"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "test.AppImage")
	if err := Download(context.Background(), srv.URL, dest, silentBar()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Errorf("content = %q, want %q", data, body)
	}
}

func TestDownload_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "test.AppImage")
	if err := Download(context.Background(), srv.URL, dest, silentBar()); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestCopy_OK(t *testing.T) {
	content := []byte("appimage content")
	src := filepath.Join(t.TempDir(), "src.AppImage")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "dst.AppImage")
	if err := Copy(src, dst); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestCopy_SourceNotFound(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dst.AppImage")
	if err := Copy("/nonexistent/path.AppImage", dst); err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}
