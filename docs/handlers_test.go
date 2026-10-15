package docs

import (
	"fmt"
	"testing"

	"github.com/imega/mygarage/store"
)

func TestRows_WithFewLines_ReturnsOnlyFilledLines(t *testing.T) {
	invoice := &store.Invoice{
		Lines: []store.InvoiceLine{
			{Quantity: "1", Name: "Oil", NetCents: 5000},
		},
	}

	actual := rows(invoice)

	if len(actual) != 1 {
		t.Fatalf("expected 1 row, got %d", len(actual))
	}
	if actual[0].Name != "Oil" {
		t.Fatalf("expected first row Oil, got %s", actual[0].Name)
	}
}

func TestRows_WithManyLines_CapsAtFourteen(t *testing.T) {
	var lines []store.InvoiceLine
	for i := 1; i <= 16; i++ {
		lines = append(lines, store.InvoiceLine{
			Quantity: "1",
			Name:     fmt.Sprintf("Line %d", i),
			NetCents: int64(i * 100),
		})
	}
	invoice := &store.Invoice{Lines: lines}

	actual := rows(invoice)

	if len(actual) != 14 {
		t.Fatalf("expected 14 rows, got %d", len(actual))
	}
	if actual[13].Name != "Line 14" {
		t.Fatalf("expected last row Line 14, got %s", actual[13].Name)
	}
}
