package export

import (
	"database/sql"
	"encoding/csv"
	"strings"

	"github.com/imega/mygarage/store"
)

func CSV(db *sql.DB) (string, error) {
	repairs, err := store.AllRepairs(db)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	w := csv.NewWriter(&b)

	header := []string{
		"date", "plate_number", "mileage", "status",
		"works", "comment",
		"invoice_number", "invoice_subtotal",
		"invoice_vat", "invoice_total",
		"created_at", "updated_at",
	}
	if err := w.Write(header); err != nil {
		return "", err
	}

	for _, r := range repairs {
		works := strings.Join(r.WorkTypes, "; ")
		number := ""
		subtotal := ""
		vat := ""
		total := ""
		invoice, err := store.GetInvoice(db, r.ID)
		if err == nil {
			number = invoice.Number
			subtotal = invoice.Subtotal()
			vat = invoice.VAT()
			total = invoice.Total()
		} else if err != sql.ErrNoRows {
			return "", err
		}

		row := []string{
			r.RepairDate.Format("2006-01-02"),
			r.PlateNumber,
			r.Mileage,
			r.Status,
			works,
			r.Comment,
			number,
			subtotal,
			vat,
			total,
			r.CreatedAt.Format("2006-01-02 15:04:05"),
			r.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}

	w.Flush()
	return b.String(), w.Error()
}
