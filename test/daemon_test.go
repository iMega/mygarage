package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/imega/mygarage/db"
	"github.com/imega/mygarage/googledrive"
	"github.com/imega/mygarage/shutdown"
	"github.com/imega/mygarage/version"
	"github.com/imega/mygarage/web"
)

func setupDaemon(t *testing.T, opts web.Options) http.Handler {
	t.Helper()

	database, err := db.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	drive := googledrive.NewConnector(
		filepath.Join(t.TempDir(), "client.json"),
		filepath.Join(t.TempDir(), "token.json"),
	)
	if opts.Shutdown == nil {
		opts.Shutdown = shutdown.New(database, drive)
	}

	r, err := web.Setup(database, "garage.db", drive, opts)
	if err != nil {
		t.Fatalf("web setup: %v", err)
	}

	return r
}

func serve(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestDaemon_DesktopOnlyRoutesAreAbsent(t *testing.T) {
	r := setupDaemon(t, web.Options{Daemon: true})

	for _, path := range []string{"/shutdown", "/settings/init-sql"} {
		if w := serve(r, http.MethodPost, path); w.Code != http.StatusNotFound {
			t.Errorf("POST %s: expected 404, got %d", path, w.Code)
		}
	}
}

func TestDaemon_PagesHideDesktopControls(t *testing.T) {
	r := setupDaemon(t, web.Options{Daemon: true})

	for _, path := range []string{"/", "/settings/data"} {
		w := serve(r, http.MethodGet, path)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: expected 200, got %d", path, w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, `action="/shutdown"`) {
			t.Errorf("GET %s: shutdown form is rendered", path)
		}
		if strings.Contains(body, `action="/settings/init-sql"`) {
			t.Errorf("GET %s: init-sql form is rendered", path)
		}
	}
}

func TestDesktop_PagesShowDesktopControls(t *testing.T) {
	r := setupDaemon(t, web.Options{})

	w := serve(r, http.MethodGet, "/settings/data")
	if !strings.Contains(w.Body.String(), `action="/shutdown"`) {
		t.Error("shutdown form is missing")
	}
	if !strings.Contains(w.Body.String(), `action="/settings/init-sql"`) {
		t.Error("init-sql form is missing")
	}
}

func TestReadyz_FollowsReadyFlag(t *testing.T) {
	var ready atomic.Bool
	r := setupDaemon(t, web.Options{Daemon: true, Ready: &ready})

	if w := serve(r, http.MethodGet, "/readyz"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("not ready: expected 503, got %d", w.Code)
	}

	ready.Store(true)
	if w := serve(r, http.MethodGet, "/readyz"); w.Code != http.StatusOK {
		t.Errorf("ready: expected 200, got %d", w.Code)
	}

	if w := serve(r, http.MethodGet, "/healthz"); w.Code != http.StatusOK {
		t.Errorf("healthz: expected 200, got %d", w.Code)
	}
}

func TestVersion_ReportsBuildSHA(t *testing.T) {
	r := setupDaemon(t, web.Options{Daemon: true})

	w := serve(r, http.MethodGet, "/version")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var got struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SHA != version.SHA {
		t.Errorf("expected sha %q, got %q", version.SHA, got.SHA)
	}
}
