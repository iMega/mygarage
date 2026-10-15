package store

import (
	"database/sql"
	"testing"

	"github.com/imega/mygarage/db"
)

func TestSaveInvoiceSettings_WithTakenNumber_SavesNextFree(t *testing.T) {
	database := setupDB(t)
	defer database.Close()

	_, err := database.Exec(
		`INSERT INTO repair_invoice
		 (repair_id, number, subtotal_cents, vat_cents,
		  total_cents, created_at, updated_at)
		 VALUES (1, '0001', 0, 0, 0, 'now', 'now')`,
	)
	if err != nil {
		t.Fatalf("insert invoice: %v", err)
	}

	err = SaveInvoiceSettings(database, "", 1, 4, DefaultVATRate)
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}

	settings, err := GetInvoiceSettings(database)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if settings.NextNumber != 2 {
		t.Fatalf("expected next number 2, got %d", settings.NextNumber)
	}
}

func TestNextInvoiceNumber_WithTakenNumber_SkipsToFree(t *testing.T) {
	database := setupDB(t)
	defer database.Close()

	_, err := database.Exec(
		`INSERT INTO repair_invoice
		 (repair_id, number, subtotal_cents, vat_cents,
		  total_cents, created_at, updated_at)
		 VALUES (1, '0001', 0, 0, 0, 'now', 'now')`,
	)
	if err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
	if err := SaveInvoiceSettings(database, "", 2, 4, DefaultVATRate); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	tx, err := database.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE invoice_settings SET next_number = 1 WHERE id = 1`,
	); err != nil {
		t.Fatalf("reset settings: %v", err)
	}

	number, err := nextInvoiceNumber(tx)
	if err != nil {
		t.Fatalf("next invoice number: %v", err)
	}
	if number != "0002" {
		t.Fatalf("expected 0002, got %s", number)
	}
}

func TestPrintNumber_WithPrefix_ReturnsNumericSuffix(t *testing.T) {
	cases := map[string]string{
		"INV-007":       "007",
		"2026-Q1-0001":  "0001",
		"2026-06-18-12": "12",
		"0003":          "0003",
		"ABC":           "ABC",
	}

	for number, expected := range cases {
		invoice := Invoice{Number: number}
		actual := invoice.PrintNumber()
		if actual != expected {
			t.Fatalf(
				"expected print number %s for %s, got %s",
				expected, number, actual,
			)
		}
	}
}

func TestInvoiceNumber_WithPlainPrefix_AddsSeparator(t *testing.T) {
	actual := InvoiceNumber("2026Q2", 5, 4)
	if actual != "2026Q2-0005" {
		t.Fatalf("expected 2026Q2-0005, got %s", actual)
	}
}

func TestInvoiceNumber_WithDelimitedPrefix_KeepsPrefix(t *testing.T) {
	actual := InvoiceNumber("INV-", 7, 3)
	if actual != "INV-007" {
		t.Fatalf("expected INV-007, got %s", actual)
	}
}

func TestSumLines_MultipliesPriceByQuantity(t *testing.T) {
	lines := []InvoiceLineInput{
		{Quantity: "2", NetCents: 5000},
		{Quantity: "1.5", NetCents: 1000},
		{Quantity: "", NetCents: 700},
	}

	actual := sumLines(lines)
	if actual != 12200 {
		t.Fatalf("expected 12200, got %d", actual)
	}
}

func setupDB(t *testing.T) *sql.DB {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("db migrate: %v", err)
	}

	return database
}
