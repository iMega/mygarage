package calendar

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type fixedReader struct {
	events []event
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func (r fixedReader) read(
	_ context.Context, _ string, _, _ time.Time,
) ([]event, error) {
	return r.events, nil
}

func TestSaveCredentials_ValidKey_RestrictedFile(t *testing.T) {
	data := credentialsJSON(t, "https://oauth2.example/token")
	path := filepath.Join(t.TempDir(), "google-calendar.json")

	email, err := SaveCredentials(path, data)
	if err != nil {
		t.Fatalf("save credentials: %v", err)
	}
	if email != "calendar@example.test" {
		t.Fatalf("expected service account email, got %s", email)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat credentials: %v", err)
	}
	actual := info.Mode().Perm()
	if actual != 0600 {
		t.Fatalf("expected credentials mode 0600, got %o", actual)
	}
}

func TestGoogleReader_ReadToday_MapsEvents(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(
		func(r *http.Request) (*http.Response, error) {
			body := ""
			switch r.URL.Path {
			case "/token":
				if err := r.ParseForm(); err != nil {
					t.Fatalf("parse token form: %v", err)
				}
				if r.Form.Get("assertion") == "" {
					t.Fatal("expected signed assertion")
				}
				body = `{"access_token":"token","expires_in":3600}`
			case "/calendar/calendars/garage@example.test/events":
				if r.Header.Get("Authorization") != "Bearer token" {
					t.Fatalf("unexpected authorization header")
				}
				if r.URL.Query().Get("orderBy") != "startTime" {
					t.Fatal("expected events ordered by start time")
				}
				body = `{"items":[` +
					`{"summary":"ABC123","status":"confirmed",` +
					`"start":{"dateTime":"2026-07-12T09:30:00+03:00"}},` +
					`{"summary":"Cancelled","status":"cancelled",` +
					`"start":{"date":"2026-07-12"}}]}`
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	)}

	path := filepath.Join(t.TempDir(), "credentials.json")
	data := credentialsJSON(t, "https://google.test/token")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	reader := newGoogleReader(path)
	reader.client = client
	reader.apiURL = "https://google.test/calendar"
	location := time.FixedZone("EEST", 3*60*60)
	from := time.Date(2026, 7, 12, 0, 0, 0, 0, location)

	events, err := reader.read(
		context.Background(), "garage@example.test",
		from, from.AddDate(0, 0, 1),
	)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Title != "ABC123" {
		t.Fatalf("expected plate title, got %s", events[0].Title)
	}
}

func TestGoogleReader_TokenExpired_RequestsNewToken(t *testing.T) {
	server := newTokenServer(t, 0)
	reader := server.reader(t)
	start := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	reader.now = func() time.Time { return start }

	server.readEvents(t, reader)
	reader.now = func() time.Time { return start.Add(2 * time.Hour) }
	server.readEvents(t, reader)

	if server.tokens != 2 {
		t.Fatalf("expected 2 token requests, got %d", server.tokens)
	}
}

func TestGoogleReader_TokenRejected_RetriesWithNewToken(t *testing.T) {
	server := newTokenServer(t, 1)
	reader := server.reader(t)

	server.readEvents(t, reader)

	if server.tokens != 2 {
		t.Fatalf("expected 2 token requests, got %d", server.tokens)
	}
}

type tokenServer struct {
	tokens   int
	rejected int
	reject   int
	client   *http.Client
}

func newTokenServer(t *testing.T, reject int) *tokenServer {
	server := &tokenServer{reject: reject}
	server.client = &http.Client{Transport: roundTripFunc(
		func(r *http.Request) (*http.Response, error) {
			status := http.StatusOK
			body := `{"items":[]}`
			switch r.URL.Path {
			case "/token":
				server.tokens++
				body = fmt.Sprintf(
					`{"access_token":"token%d","expires_in":3600}`,
					server.tokens,
				)
			case "/calendar/calendars/garage@example.test/events":
				if server.rejected < server.reject {
					server.rejected++
					status = http.StatusUnauthorized
					body = "invalid credentials"
				}
			default:
				t.Fatalf("unexpected request %s", r.URL.Path)
			}
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		},
	)}
	return server
}

func (s *tokenServer) reader(t *testing.T) *googleReader {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	data := credentialsJSON(t, "https://google.test/token")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	reader := newGoogleReader(path)
	reader.client = s.client
	reader.apiURL = "https://google.test/calendar"
	return reader
}

func (s *tokenServer) readEvents(t *testing.T, reader *googleReader) {
	t.Helper()
	from := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	_, err := reader.read(
		context.Background(), "garage@example.test",
		from, from.AddDate(0, 0, 1),
	)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
}

func TestPlateFromTitle_WithNotes_ExtractsPlate(t *testing.T) {
	tests := map[string]string{
		"ABC123":               "ABC123",
		"ABC 123 замена масла": "ABC123",
		"Срочно: abc-123 диагностика": "ABC123",
		"Франция 7149 VV 33 клиент":   "7149VV33",
		"Франция WW-412-DX клиент":    "WW412DX",
		"Франция 2509 RL 24 клиент":   "2509RL24",
		"Франция HJ-159-CQ клиент":    "HJ159CQ",
		"Германия KLE QH 426 клиент":  "KLEQH426",
		"Германия F LF 24 клиент":     "FLF24",
		"Германия BN CM 192 клиент":   "BNCM192",
		"Греция AXI-8995 клиент":      "AXI8995",
		"Греция IAA-7556 клиент":      "IAA7556",
		"Греция NMI-9763 клиент":      "NMI9763",
		"Британия LK53 WUO клиент":    "LK53WUO",
		"Украина AA 1700 YE клиент":   "AA1700YE",
		"Украина AA-1700-YE клиент":   "AA1700YE",
		"Украина AA1700YE клиент":     "AA1700YE",
		"Замена масла в 10:30":        "",
	}
	for title, expected := range tests {
		actual := plateFromTitle(title)
		if actual != expected {
			t.Errorf(
				"plateFromTitle(%q): expected %q, got %q",
				title, expected, actual,
			)
		}
	}
}

func TestAgendaToday_KnownVehicle_BuildsVisit(t *testing.T) {
	database := calendarDB(t)
	defer database.Close()
	if err := SaveSettings(database, true, "garage@example.test"); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	now := time.Date(2026, 7, 12, 8, 0, 0, 0, time.Local)
	_, err := database.Exec(
		`INSERT INTO vehicles
		 (id, plate_number, created_at, updated_at)
		 VALUES (7, 'ABC123', ?, ?)`,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("insert vehicle: %v", err)
	}

	agenda := &Agenda{
		db: database,
		reader: fixedReader{events: []event{{
			Title: "Замена масла для abc-123",
			Start: time.Date(
				2026, 7, 12, 9, 30, 0, 0, time.Local,
			),
		}}},
		now: func() time.Time { return now },
	}

	visits, err := agenda.Today(context.Background())
	if err != nil {
		t.Fatalf("load agenda: %v", err)
	}
	if len(visits) != 1 {
		t.Fatalf("expected one visit, got %d", len(visits))
	}
	actual := visits[0]
	if !actual.Known || actual.VehicleID != 7 {
		t.Fatalf("expected known vehicle, got %+v", actual)
	}
	if actual.Time != "09:30" {
		t.Fatalf("expected visit time 09:30, got %s", actual.Time)
	}
	if actual.Plate != "ABC123" {
		t.Fatalf("expected stored plate format, got %s", actual.Plate)
	}
	if !strings.Contains(actual.CreateURL, "plate=ABC123") {
		t.Fatalf("expected prefilled create URL, got %s", actual.CreateURL)
	}
}

func TestAgendaToday_RepairOnEventDate_HidesVisit(t *testing.T) {
	database := calendarDB(t)
	defer database.Close()
	if err := SaveSettings(database, true, "garage@example.test"); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	now := time.Date(2026, 7, 12, 8, 0, 0, 0, time.Local)
	stamp := now.Format(time.RFC3339)
	_, err := database.Exec(
		`INSERT INTO vehicles
		 (id, plate_number, created_at, updated_at)
		 VALUES (7, 'ABC123', ?, ?);
		 INSERT INTO repairs
		 (id, vehicle_id, repair_date, mileage, status, comment,
		  created_at, updated_at)
		 VALUES (9, 7, '2026-07-12', '90000', 'in_progress', '', ?, ?)`,
		stamp, stamp, stamp, stamp,
	)
	if err != nil {
		t.Fatalf("insert repair: %v", err)
	}

	agenda := &Agenda{
		db: database,
		reader: fixedReader{events: []event{{
			Title: "ABC123 плановое обслуживание",
			Start: time.Date(
				2026, 7, 12, 9, 30, 0, 0, time.Local,
			),
		}}},
		now: func() time.Time { return now },
	}

	visits, err := agenda.Today(context.Background())
	if err != nil {
		t.Fatalf("load agenda: %v", err)
	}
	if len(visits) != 0 {
		t.Fatalf("expected repaired visit to be hidden, got %d", len(visits))
	}
}

func TestAgendaToday_UnknownVehicle_UsesCompactPlate(t *testing.T) {
	database := calendarDB(t)
	defer database.Close()
	if err := SaveSettings(database, true, "garage@example.test"); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	now := time.Date(2026, 7, 12, 8, 0, 0, 0, time.Local)
	agenda := &Agenda{
		db: database,
		reader: fixedReader{events: []event{{
			Title: "Клиент LK53 WUO диагностика",
			Start: time.Date(
				2026, 7, 12, 11, 0, 0, 0, time.Local,
			),
		}}},
		now: func() time.Time { return now },
	}

	visits, err := agenda.Today(context.Background())
	if err != nil {
		t.Fatalf("load agenda: %v", err)
	}
	if len(visits) != 1 {
		t.Fatalf("expected one visit, got %d", len(visits))
	}
	actual := visits[0]
	if actual.Plate != "LK53WUO" {
		t.Fatalf("expected compact plate, got %s", actual.Plate)
	}
	if !strings.Contains(actual.CreateURL, "plate=LK53WUO") {
		t.Fatalf("expected compact create URL, got %s", actual.CreateURL)
	}
}

func credentialsJSON(t *testing.T, tokenURI string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal RSA key: %v", err)
	}
	value := map[string]string{
		"client_email": "calendar@example.test",
		"private_key": string(pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: encoded,
		})),
		"token_uri": tokenURI,
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	return data
}

func calendarDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open(
		"sqlite", filepath.Join(t.TempDir(), "calendar.db"),
	)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	schema := `
	CREATE TABLE calendar_settings (
	    id INTEGER PRIMARY KEY,
	    enabled INTEGER NOT NULL,
	    calendar_id TEXT NOT NULL,
	    created_at TEXT NOT NULL,
	    updated_at TEXT NOT NULL
	);
	CREATE TABLE vehicles (
	    id INTEGER PRIMARY KEY,
	    plate_number TEXT NOT NULL,
	    created_at TEXT NOT NULL,
	    updated_at TEXT NOT NULL
	);
	CREATE TABLE repairs (
	    id INTEGER PRIMARY KEY,
	    vehicle_id INTEGER NOT NULL,
	    repair_date TEXT NOT NULL,
	    mileage TEXT NOT NULL,
	    status TEXT NOT NULL,
	    comment TEXT,
	    created_at TEXT NOT NULL,
	    updated_at TEXT NOT NULL
	);
	CREATE TABLE repair_work_types (
	    repair_id INTEGER NOT NULL,
	    work_type_id INTEGER NOT NULL
	);
	CREATE TABLE work_types (
	    id INTEGER PRIMARY KEY,
	    name TEXT NOT NULL,
	    sort_order INTEGER NOT NULL DEFAULT 0
	);`
	if _, err := database.Exec(schema); err != nil {
		database.Close()
		t.Fatalf("create schema: %v", err)
	}
	return database
}
