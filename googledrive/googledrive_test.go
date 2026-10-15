package googledrive

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSaveClient_DesktopClient_RestrictedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.json")

	clientID, err := SaveClient(path, clientJSON("https://token.test"))
	if err != nil {
		t.Fatalf("save client: %v", err)
	}
	if clientID != "client-id" {
		t.Fatalf("expected client-id, got %s", clientID)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat client: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}
}

func TestValidateClient_WebClient_Rejected(t *testing.T) {
	data := []byte(`{"web":{"client_id":"id","client_secret":"secret"}}`)

	_, err := ValidateClient(data)

	if err == nil || !strings.Contains(err.Error(), "Desktop app") {
		t.Fatalf("expected Desktop app error, got %v", err)
	}
}

func TestConnector_CompleteFlow_StoresRefreshToken(t *testing.T) {
	var verifier string
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if r.Form.Get("code") != "code" {
				t.Fatalf("unexpected code %s", r.Form.Get("code"))
			}
			verifier = r.Form.Get("code_verifier")
			_, _ = io.WriteString(
				w, `{"access_token":"access","refresh_token":"refresh"}`,
			)
		},
	))
	defer server.Close()
	connector := newTestConnector(t, server.URL)

	authURL, err := connector.AuthURL("http://127.0.0.1:1/callback")
	if err != nil {
		t.Fatalf("auth url: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	query := parsed.Query()
	if query.Get("scope") != driveScope {
		t.Fatalf("unexpected scope %s", query.Get("scope"))
	}
	err = connector.Complete(context.Background(), query.Get("state"), "code")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if challenge != query.Get("code_challenge") {
		t.Fatal("code verifier does not match challenge")
	}
	if !Connected(connector.tokenPath) {
		t.Fatal("expected connected state")
	}
}

func TestConnector_CompleteUnknownState_Rejected(t *testing.T) {
	connector := newTestConnector(t, "http://127.0.0.1:1")

	err := connector.Complete(context.Background(), "unknown", "code")

	if err == nil {
		t.Fatal("expected error for unknown state")
	}
}

func TestSession_Upload_CreatesFolderUploadsAndRotates(t *testing.T) {
	var mu sync.Mutex
	var uploaded string
	var deleted []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			if r.Header.Get("Authorization") != "Bearer access" &&
				r.URL.Path != "/session" {
				t.Fatalf("missing authorization on %s", r.URL.Path)
			}
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/files/old":
				w.WriteHeader(http.StatusNotFound)
			case r.Method == http.MethodPost && r.URL.Path == "/api/files":
				_, _ = io.WriteString(w, `{"id":"folder"}`)
			case r.Method == http.MethodPost && r.URL.Path == "/upload/files":
				var meta struct {
					Parents []string `json:"parents"`
				}
				_ = json.NewDecoder(r.Body).Decode(&meta)
				if len(meta.Parents) != 1 || meta.Parents[0] != "folder" {
					t.Fatalf("unexpected parents %v", meta.Parents)
				}
				w.Header().Set("Location", server.URL+"/session")
			case r.Method == http.MethodPut && r.URL.Path == "/session":
				data, _ := io.ReadAll(r.Body)
				uploaded = string(data)
			case r.Method == http.MethodGet && r.URL.Path == "/api/files":
				_, _ = io.WriteString(w, `{"files":[`+
					`{"id":"c","name":"garage-3.db"},`+
					`{"id":"a","name":"garage-1.db"},`+
					`{"id":"b","name":"garage-2.db"}]}`)
			case r.Method == http.MethodDelete:
				deleted = append(deleted, strings.TrimPrefix(
					r.URL.Path, "/api/files/",
				))
			default:
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			}
		},
	))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "garage-3.db")
	if err := os.WriteFile(path, []byte("snapshot"), 0600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	s := &session{
		connector: newTestConnector(t, server.URL),
		token:     "access",
		apiURL:    server.URL + "/api",
		uploadURL: server.URL + "/upload",
	}

	folderID, err := s.upload(
		context.Background(), "old", path, "garage-3.db", 2,
	)

	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if folderID != "folder" {
		t.Fatalf("expected new folder, got %s", folderID)
	}
	if uploaded != "snapshot" {
		t.Fatalf("unexpected uploaded content %q", uploaded)
	}
	if len(deleted) != 1 || deleted[0] != "a" {
		t.Fatalf("expected oldest file deleted, got %v", deleted)
	}
}

func newTestConnector(t *testing.T, tokenURI string) *Connector {
	t.Helper()
	dir := t.TempDir()
	clientPath := filepath.Join(dir, "client.json")
	if _, err := SaveClient(clientPath, clientJSON(tokenURI)); err != nil {
		t.Fatalf("save client: %v", err)
	}
	return NewConnector(clientPath, filepath.Join(dir, "token.json"))
}

func clientJSON(tokenURI string) []byte {
	return []byte(`{"installed":{"client_id":"client-id",` +
		`"client_secret":"secret","token_uri":"` + tokenURI + `"}}`)
}
