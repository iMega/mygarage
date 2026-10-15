package test

import (
	"bytes"
	"database/sql"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imega/mygarage/appdir"
	"github.com/imega/mygarage/backup"
	"github.com/imega/mygarage/db"
	"github.com/imega/mygarage/googledrive"
	"github.com/imega/mygarage/store"
	"github.com/imega/mygarage/web"
)

func TestMain(m *testing.M) {
	os.Chdir("..")
	code := m.Run()
	os.Exit(code)
}

func setupTest(t *testing.T) (*sql.DB, http.Handler, func()) {
	dbPath := "test.db"

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}

	if err := db.Migrate(database); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	r, err := web.Setup(
		database, dbPath,
		googledrive.NewConnector(
			filepath.Join(t.TempDir(), "client.json"),
			filepath.Join(t.TempDir(), "token.json"),
		),
		web.Options{},
	)
	if err != nil {
		t.Fatalf("web setup: %v", err)
	}

	cleanup := func() {
		database.Close()
		os.Remove(dbPath)
	}

	return database, r, cleanup
}

func TestHomePageLoads(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "myGarage") {
		t.Error("home page should contain app name")
	}

	if !strings.Contains(body, "Номер автомобиля") {
		t.Error("home page should contain search input")
	}
}

func TestSearchVehicleNotFound(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"/vehicles/search?plate=ABC123",
		nil,
	)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("expected redirect status, got %d", w.Code)
	}

	location := w.Header().Get("Location")
	if location != "/repairs/new?plate=ABC123" {
		t.Fatalf("expected redirect to new repair, got %s", location)
	}

	req = httptest.NewRequest(http.MethodGet, location, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected new repair page status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "ABC123") {
		t.Error("should show searched plate")
	}
	if !strings.Contains(body, "Создать новый ремонт") {
		t.Error("should show first repair form")
	}
	if !strings.Contains(
		body,
		`<option value="in_progress" selected>В работе</option>`,
	) {
		t.Error("new repair should default to in progress")
	}
	if !strings.Contains(body, "Открыть настройки работ") {
		t.Error("new repair page should link to work settings when list is empty")
	}
	if !strings.Contains(body, "Добавьте активные работы") {
		t.Error("new repair page should explain how to fill the work list")
	}
	if strings.Contains(body, `<div class="summary-grid`) {
		t.Error("new repair page should not show current queue")
	}
}

func TestCreateAndFindRepair(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	form := url.Values{}
	form.Add("plate", "ABC123")
	form.Add("date", "2026-06-09")
	form.Add("mileage", "86400 km")
	form.Add("status", "in_progress")
	form.Add("comment", "Test repair")

	req := httptest.NewRequest(
		http.MethodPost,
		"/repairs",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf(
			"expected redirect status, got %d",
			w.Code,
		)
	}

	location := w.Header().Get("Location")
	req2 := httptest.NewRequest(
		http.MethodGet,
		location,
		nil,
	)
	w2 := httptest.NewRecorder()

	r.ServeHTTP(w2, req2)

	body := w2.Body.String()
	if !strings.Contains(body, "ABC123") {
		t.Error("should find created vehicle")
	}
	if !strings.Contains(body, "История ремонтов") {
		t.Error("should show repair history")
	}
	if !strings.Contains(body, "Test repair") {
		t.Error("should show repair comment")
	}
	if !strings.Contains(body, "86400") {
		t.Error("should show repair mileage")
	}
	if !strings.Contains(body, "В работе") {
		t.Error("should show repair status")
	}
	if !strings.Contains(body, "Добавить ремонт") {
		t.Error("should show add repair button")
	}
}

func TestEditRepair_ChangesVehicle(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	rid := createRepairWithInvoice(t, database, r)

	req := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/repairs/%d/edit", rid),
		nil,
	)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected edit page status 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `id="editPlate"`) {
		t.Fatal("edit form should show plate edit button")
	}

	form := url.Values{}
	form.Add("plate", "XYZ 999")
	form.Add("date", "2026-06-10")
	form.Add("mileage", "91000")
	form.Add("status", "ready")
	form.Add("comment", "Moved repair")
	form.Add("line_work_type_id", "")
	form.Add("line_name", "Diagnostics")
	form.Add("line_quantity", "1")
	form.Add("line_net", "100.00")

	req = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/repairs/%d", rid),
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	var vid int64
	q := `SELECT id FROM vehicles WHERE plate_number = ?`
	if err := database.QueryRow(q, "XYZ999").Scan(&vid); err != nil {
		t.Fatalf("load new vehicle: %v", err)
	}

	location := w.Header().Get("Location")
	expected := fmt.Sprintf("/vehicles/%d", vid)
	if location != expected {
		t.Fatalf("expected redirect to %s, got %s", expected, location)
	}

	var actual int64
	q = `SELECT vehicle_id FROM repairs WHERE id = ?`
	if err := database.QueryRow(q, rid).Scan(&actual); err != nil {
		t.Fatalf("load repair vehicle: %v", err)
	}
	if actual != vid {
		t.Fatalf("expected repair vehicle %d, got %d", vid, actual)
	}
}

func TestInvoiceCreateEditAndPrint(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	saveSettings(t, r)
	rid := createRepairWithInvoice(t, database, r)

	invoice := loadInvoice(t, database, rid)
	if invoice.number != "INV-007" {
		t.Errorf("expected invoice number INV-007, got %s", invoice.number)
	}
	if invoice.subtotal != 10000 {
		t.Errorf("expected subtotal 10000, got %d", invoice.subtotal)
	}
	if invoice.vat != 1900 {
		t.Errorf("expected vat 1900, got %d", invoice.vat)
	}
	if invoice.total != 11900 {
		t.Errorf("expected total 11900, got %d", invoice.total)
	}

	form := url.Values{}
	form.Add("date", "2026-06-09")
	form.Add("mileage", "91000 km")
	form.Add("status", "paid")
	form.Add("comment", "Updated")
	form.Add("line_work_type_id", "")
	form.Add("line_name", "Oil")
	form.Add("line_quantity", "2")
	form.Add("line_net", "50.00")

	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/repairs/%d", rid),
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	updated := loadInvoice(t, database, rid)
	if updated.number != "INV-007" {
		t.Errorf("expected invoice number to stay, got %s", updated.number)
	}
	if updated.subtotal != 10000 {
		t.Errorf("expected updated subtotal 10000, got %d", updated.subtotal)
	}

	repair, err := store.GetRepair(database, rid)
	if err != nil {
		t.Fatalf("get repair: %v", err)
	}
	if repair.Mileage != "91000" {
		t.Fatalf("expected mileage 91000, got %s", repair.Mileage)
	}
	if repair.Status != "paid" {
		t.Fatalf("expected status paid, got %s", repair.Status)
	}
	assertPageContains(
		t, r,
		fmt.Sprintf("/vehicles/%d", repair.VehicleID),
		"1 x Счёт", "2 x Счёт",
	)

	form = url.Values{}
	form.Add("name", "My Garage Ltd")
	form.Add("address", "Main street")
	form.Add("fpa_number", "FPA-900")
	form.Add("aft_number", "AFT-001")
	form.Add("phone", "+357000000")
	form.Add("email", "info@example.test")
	form.Add("technician_name", "Test Technician")
	form.Add("technician_speciality", "TEST SPECIALITY")
	form.Add("technician_registry_number", "777")
	form.Add("prefix", "INV-")
	form.Add("next_number", "7")
	form.Add("padding", "3")
	form.Add("vat_rate", "19")

	req = httptest.NewRequest(
		http.MethodPost,
		"/settings/company",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected settings redirect, got %d", w.Code)
	}

	var next int64
	q := `SELECT next_number FROM invoice_settings WHERE id = 1`
	if err := database.QueryRow(q).Scan(&next); err != nil {
		t.Fatalf("load next number: %v", err)
	}
	if next != 8 {
		t.Fatalf("expected adjusted next number 8, got %d", next)
	}

	assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/certificate", rid),
		"ΒΕΒΑΙΩΣΗ", "ABC123", "Test Technician",
		"TEST SPECIALITY", "777",
	)
	body := assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/invoice", rid),
		"ΤΙΜΟΛΟΓΙΟ", "ΠΟΣΟΤΗΣ", "007",
		"ΦΠΑ:", "FPA-900", "ΑΦΤ:", "AFT-001",
		"VAT 19 %", "My Garage Ltd", "Main street", "50.00", "100",
	)
	rows := strings.Count(body, `class="item-row"`)
	if rows != 2 {
		t.Errorf("expected 2 invoice item rows, got %d", rows)
	}
	oneCopy := assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/invoice?copies=1", rid),
		"ΤΙΜΟΛΟΓΙΟ", "ΠΟΣΟΤΗΣ",
	)
	rows = strings.Count(oneCopy, `class="item-row"`)
	if rows != 1 {
		t.Errorf("expected 1 invoice item row, got %d", rows)
	}

	preview := assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/invoice", rid),
		"Предпросмотр документа",
	)
	if strings.Contains(preview, "window.print();") {
		t.Fatal("plain invoice preview should not auto print")
	}
	assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/invoice?print=1", rid),
		"window.print();",
	)
	assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/certificate?print=1", rid),
		"window.print();",
	)
	documents := assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/documents", rid),
		"Предпросмотр документов", "ΒΕΒΑΙΩΣΗ", "ΤΙΜΟΛΟΓΙΟ",
		"break-after: page", "Печать",
	)
	if strings.Contains(documents, "window.print();") {
		t.Fatal("plain documents preview should not auto print")
	}
	assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/documents?print=1", rid),
		"window.print();",
	)
}

func TestInvoiceLinesAreLimitedToPrintableRows(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	form := url.Values{}
	form.Add("plate", "LIMIT14")
	form.Add("date", "2026-06-09")
	form.Add("status", "ready")
	for i := 0; i < 16; i++ {
		form.Add("line_work_type_id", "")
		form.Add("line_name", fmt.Sprintf("Line %02d", i+1))
		form.Add("line_quantity", "1")
		form.Add("line_net", "1.00")
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/repairs",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	var rid int64
	if err := database.QueryRow(
		`SELECT id FROM repairs ORDER BY id DESC LIMIT 1`,
	).Scan(&rid); err != nil {
		t.Fatalf("load repair id: %v", err)
	}

	if count := invoiceLineCount(t, database, rid); count != 14 {
		t.Fatalf("expected 14 invoice lines, got %d", count)
	}
}

func TestHomeQueueAndMarkPaid(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	saveSettings(t, r)
	rid := createRepairWithInvoice(t, database, r)

	body := assertPageContains(
		t, r, "/",
		"1111", "(история)", `href="/history"`,
		"ABC123", "119.00",
		"table-hover",
		"Βεβαίωση", "1 x Счёт", "2 x Счёт", "Документы",
		"Оплачено",
	)
	if !strings.Contains(body, "К оплате") {
		t.Fatal("expected unpaid repairs summary")
	}

	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/repairs/%d/pay", rid),
		nil,
	)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	repair, err := store.GetRepair(database, rid)
	if err != nil {
		t.Fatalf("get repair: %v", err)
	}
	if repair.Status != "paid" {
		t.Fatalf("expected status paid, got %s", repair.Status)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if strings.Contains(w.Body.String(), "ABC123") {
		t.Fatal("paid repair should not be shown in current queue")
	}
}

func TestRepairHistoryListsInvoicesAndPaginates(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	saveSettings(t, r)
	rid := createRepairWithInvoice(t, database, r)

	newID, err := store.CreateVehicle(database, "NEW001")
	if err != nil {
		t.Fatalf("create newer vehicle: %v", err)
	}
	for i := 0; i < 49; i++ {
		_, err := store.CreateRepair(
			database, newID, "2026-07-01", "", "paid", "", nil, nil,
		)
		if err != nil {
			t.Fatalf("create newer repair: %v", err)
		}
	}

	oldID, err := store.CreateVehicle(database, "OLD001")
	if err != nil {
		t.Fatalf("create older vehicle: %v", err)
	}
	_, err = store.CreateRepair(
		database, oldID, "2026-01-01", "", "paid", "", nil, nil,
	)
	if err != nil {
		t.Fatalf("create older repair: %v", err)
	}

	body := assertPageContains(
		t, r, "/history",
		"История ремонтов", "NEW001", "ABC123", "INV-007",
		fmt.Sprintf(`/repairs/%d/invoice`, rid),
		"Страница 1 из 2", `href="/history?page=2"`,
	)
	if strings.Contains(body, "OLD001") {
		t.Fatal("first history page should not contain the oldest repair")
	}
	if strings.Index(body, "NEW001") > strings.Index(body, "ABC123") {
		t.Fatal("newer repairs should be listed first")
	}

	body = assertPageContains(
		t, r, "/history?page=2",
		"OLD001", "Страница 2 из 2", `href="/history?page=1"`,
	)
	if strings.Contains(body, "ABC123") {
		t.Fatal("second history page should only contain remaining repairs")
	}
}

func TestHomeQueueWithoutInvoice_HidesInvoiceAndPay(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	form := url.Values{}
	form.Add("plate", "NOINV1")
	form.Add("date", "2026-06-09")
	form.Add("mileage", "90000")
	form.Add("status", "ready")
	form.Add("comment", "Check without invoice")

	req := httptest.NewRequest(
		http.MethodPost,
		"/repairs",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	body := assertPageContains(t, r, "/", "NOINV1", "Открыть")
	if strings.Contains(body, "/invoice") {
		t.Fatal("repair without invoice should not show invoice action")
	}
	if strings.Contains(body, "/documents") {
		t.Fatal("repair without invoice should not show documents action")
	}
	if strings.Contains(body, "/pay") {
		t.Fatal("repair without invoice total should not show pay action")
	}
	if strings.Contains(body, "Βεβαίωση") {
		t.Fatal("repair without invoice should not show certificate action")
	}
}

func TestInvoiceUsesConfiguredVATRate(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	form := url.Values{}
	form.Add("name", "My Garage Ltd")
	form.Add("address", "Main street")
	form.Add("fpa_number", "FPA-900")
	form.Add("aft_number", "AFT-001")
	form.Add("phone", "+357000000")
	form.Add("email", "info@example.test")
	form.Add("technician_name", "Test Technician")
	form.Add("technician_speciality", "TEST SPECIALITY")
	form.Add("technician_registry_number", "777")
	form.Add("prefix", "INV-")
	form.Add("next_number", "7")
	form.Add("padding", "3")
	form.Add("vat_rate", "20")

	req := httptest.NewRequest(
		http.MethodPost,
		"/settings/company",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected settings redirect, got %d", w.Code)
	}

	rid := createRepairWithInvoice(t, database, r)
	invoice := loadInvoice(t, database, rid)
	if invoice.vat != 2000 {
		t.Fatalf("expected VAT 2000 with 20%% rate, got %d", invoice.vat)
	}
	if invoice.total != 12000 {
		t.Fatalf("expected total 12000 with 20%% rate, got %d", invoice.total)
	}

	body := assertPageContains(
		t, r,
		fmt.Sprintf("/repairs/%d/invoice", rid),
		"VAT 20 %",
	)
	if !strings.Contains(body, "120") {
		t.Fatal("expected invoice page to show total with configured VAT")
	}
}

func TestExportCSVIncludesInvoiceFields(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	saveSettings(t, r)
	createRepairWithInvoice(t, nil, r)

	req := httptest.NewRequest(
		http.MethodGet,
		"/export/repairs.csv",
		nil,
	)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "invoice_number") {
		t.Error("should include invoice_number header")
	}
	if !strings.Contains(body, "INV-007") {
		t.Error("should include invoice number")
	}
	if !strings.Contains(body, "100.00") {
		t.Error("should include invoice subtotal")
	}
}

func TestWorkTypesPage(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/work-types", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Работы") {
		t.Error("should show work types page title")
	}
	if !strings.Contains(body, "Документы") {
		t.Error("should show settings side menu")
	}
}

func TestSettingsDocumentsPageShowsAssetUploads(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/settings/company", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Загрузить подпись") {
		t.Error("should show signature upload action")
	}
	if !strings.Contains(body, "Загрузить штамп") {
		t.Error("should show stamp upload action")
	}
	if strings.Contains(body, "URL подписи") ||
		strings.Contains(body, "URL штампа") {
		t.Error("should not show manual signature or stamp URL fields")
	}
}

func TestSettingsDataPage(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/settings/data", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Скачать CSV") {
		t.Error("should show CSV export action")
	}
	if !strings.Contains(body, "Создать резервную копию") {
		t.Error("should show backup action")
	}
	if !strings.Contains(body, "Загрузить init.sql") {
		t.Error("should show init.sql upload action")
	}
	if !strings.Contains(body, "Файлы документов") {
		t.Error("should show document assets directory")
	}
	if !strings.Contains(body, "Локальная копия") {
		t.Error("should show local backup switch")
	}
	if !strings.Contains(body, "Google Drive") {
		t.Error("should show Google Drive backup switch")
	}
}

func TestSaveBackupSettings_DriveNotConnected_Rejected(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	form := url.Values{}
	form.Set("local", "1")
	form.Set("drive", "1")
	req := httptest.NewRequest(
		http.MethodPost, "/settings/backup",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestSaveBackupSettings_LocalOff_Persisted(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodPost, "/settings/backup", strings.NewReader(""),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", w.Code)
	}
	settings, err := backup.GetSettings(database)
	if err != nil {
		t.Fatalf("get backup settings: %v", err)
	}
	if settings.Local || settings.Drive {
		t.Fatalf("expected both destinations off, got %+v", settings)
	}
}

func TestSettingsAccessPage(t *testing.T) {
	database, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/settings/access", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, expected := range []string{
		"Доступы", "Google Calendar", "Calendar ID",
		"JSON-ключ service account", "создание проекта Google Cloud",
		"мастер доступа Calendar API", "Поиск автомобиля",
		`class="data-section"`,
		"Просмотр всех сведений",
		"Интеграция календаря",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("access settings should contain %q", expected)
		}
	}
	if strings.Contains(body, "На главную") {
		t.Fatal("access settings should use the shared settings header")
	}

	form := url.Values{}
	form.Set("calendar_id", "garage@example.test")
	req = httptest.NewRequest(
		http.MethodPost,
		"/settings/calendar",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	var calendarID string
	err := database.QueryRow(
		`SELECT calendar_id FROM calendar_settings WHERE id = 1`,
	).Scan(&calendarID)
	if err != nil {
		t.Fatalf("load calendar settings: %v", err)
	}
	if calendarID != "garage@example.test" {
		t.Fatalf("expected saved calendar ID, got %s", calendarID)
	}
}

func TestUploadInitSQL(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "seed-data.sql")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("SELECT 1;")); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/settings/init-sql",
		&body,
	)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	path, err := appdir.InitSQL()
	if err != nil {
		t.Fatalf("init sql path: %v", err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read init sql: %v", err)
	}
	if string(data) != "SELECT 1;" {
		t.Fatalf("unexpected init sql content: %s", data)
	}
}

func TestExportCSV(t *testing.T) {
	_, r, cleanup := setupTest(t)
	defer cleanup()

	req := httptest.NewRequest(
		http.MethodGet,
		"/export/repairs.csv",
		nil,
	)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/csv") {
		t.Errorf("expected CSV content type, got %s", contentType)
	}
}

type invoiceRow struct {
	number   string
	subtotal int64
	vat      int64
	total    int64
}

func saveSettings(t *testing.T, r http.Handler) {
	t.Helper()

	form := url.Values{}
	form.Add("name", "My Garage Ltd")
	form.Add("address", "Main street")
	form.Add("fpa_number", "FPA-900")
	form.Add("aft_number", "AFT-001")
	form.Add("phone", "+357000000")
	form.Add("email", "info@example.test")
	form.Add("prefix", "INV-")
	form.Add("next_number", "7")
	form.Add("padding", "3")
	form.Add("vat_rate", "19")

	req := httptest.NewRequest(
		http.MethodPost,
		"/settings/company",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}
}

func createRepairWithInvoice(
	t *testing.T, db *sql.DB, r http.Handler,
) int64 {
	t.Helper()

	form := url.Values{}
	form.Add("plate", "ABC123")
	form.Add("date", "2026-06-09")
	form.Add("mileage", "90000 km")
	form.Add("status", "ready")
	form.Add("comment", "Test repair")
	form.Add("line_work_type_id", "")
	form.Add("line_name", "Diagnostics")
	form.Add("line_quantity", "1")
	form.Add("line_net", "100.00")

	req := httptest.NewRequest(
		http.MethodPost,
		"/repairs",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", w.Code)
	}

	if db == nil {
		return 0
	}

	var rid int64
	q := `SELECT id FROM repairs ORDER BY id DESC LIMIT 1`
	if err := db.QueryRow(q).Scan(&rid); err != nil {
		t.Fatalf("load repair id: %v", err)
	}

	return rid
}

func loadInvoice(t *testing.T, db *sql.DB, rid int64) invoiceRow {
	t.Helper()

	q := `SELECT number, subtotal_cents, vat_cents, total_cents
	      FROM repair_invoice WHERE repair_id = ?`
	var row invoiceRow
	err := db.QueryRow(q, rid).Scan(
		&row.number, &row.subtotal, &row.vat, &row.total,
	)
	if err != nil {
		t.Fatalf("load invoice: %v", err)
	}

	return row
}

func invoiceLineCount(t *testing.T, db *sql.DB, rid int64) int {
	t.Helper()

	q := `SELECT COUNT(*)
	      FROM repair_invoice_lines ril
	      JOIN repair_invoice ri ON ri.id = ril.invoice_id
	      WHERE ri.repair_id = ?`
	var count int
	if err := db.QueryRow(q, rid).Scan(&count); err != nil {
		t.Fatalf("count invoice lines: %v", err)
	}
	return count
}

func assertPageContains(
	t *testing.T, r http.Handler, path string, values ...string,
) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 for %s, got %d", path, w.Code)
	}

	body := w.Body.String()
	for _, value := range values {
		if !strings.Contains(body, value) {
			t.Fatalf("expected %s to contain %s", path, value)
		}
	}

	return body
}
