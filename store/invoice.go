package store

import (
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const DefaultVATRate = 19

type Invoice struct {
	ID            int64
	RepairID      int64
	Number        string
	SubtotalCents int64
	VATCents      int64
	TotalCents    int64
	Lines         []InvoiceLine
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type InvoiceLine struct {
	ID         int64
	InvoiceID  int64
	WorkTypeID int64
	Kind       string
	Name       string
	Quantity   string
	NetCents   int64
	SortOrder  int
}

type InvoiceLineInput struct {
	WorkTypeID int64
	Kind       string
	Name       string
	Quantity   string
	NetCents   int64
}

type InvoiceSettings struct {
	Prefix     string
	NextNumber int64
	Padding    int
	VATRate    int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func GetInvoice(db *sql.DB, rid int64) (*Invoice, error) {
	q := `SELECT id, repair_id, number, subtotal_cents,
	             vat_cents, total_cents, created_at, updated_at
	      FROM repair_invoice WHERE repair_id = ?`

	var inv Invoice
	var created, updated string
	err := db.QueryRow(q, rid).Scan(
		&inv.ID, &inv.RepairID, &inv.Number,
		&inv.SubtotalCents, &inv.VATCents, &inv.TotalCents,
		&created, &updated,
	)
	if err != nil {
		return nil, err
	}

	inv.CreatedAt, _ = time.Parse(time.RFC3339, created)
	inv.UpdatedAt, _ = time.Parse(time.RFC3339, updated)

	lines, err := loadLines(db, inv.ID)
	if err != nil {
		return nil, err
	}
	inv.Lines = lines

	return &inv, nil
}

func GetInvoiceSettings(db *sql.DB) (*InvoiceSettings, error) {
	if err := ensureInvoiceSettings(db); err != nil {
		return nil, err
	}

	q := `SELECT prefix, next_number, padding, vat_rate,
	             created_at, updated_at
	      FROM invoice_settings WHERE id = 1`

	var s InvoiceSettings
	var created, updated string
	err := db.QueryRow(q).Scan(
		&s.Prefix, &s.NextNumber, &s.Padding, &s.VATRate,
		&created, &updated,
	)
	if err != nil {
		return nil, err
	}

	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updated)

	return &s, nil
}

func SaveInvoiceSettings(
	db *sql.DB, prefix string, next int64, padding int, rate int,
) error {
	if next < 1 {
		next = 1
	}
	if padding < 1 {
		padding = 1
	}
	if rate < 0 {
		rate = DefaultVATRate
	}

	next, err := nextFreeInvoiceNumber(db, prefix, next, padding)
	if err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)
	q := `INSERT INTO invoice_settings
	      (id, prefix, next_number, padding, vat_rate,
	       created_at, updated_at)
	      VALUES (1, ?, ?, ?, ?, ?, ?)
	      ON CONFLICT(id) DO UPDATE SET
	          prefix = excluded.prefix,
	          next_number = excluded.next_number,
	          padding = excluded.padding,
	          vat_rate = excluded.vat_rate,
	          updated_at = excluded.updated_at`

	_, err = db.Exec(q, prefix, next, padding, rate, now, now)
	return err
}

func saveInvoice(tx *sql.Tx, rid int64, lines []InvoiceLineInput) error {
	existing, err := getInvoiceID(tx, rid)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	isNew := err == sql.ErrNoRows

	if len(lines) == 0 && isNew {
		return nil
	}

	subtotal := sumLines(lines)
	rate, err := vatRateTx(tx)
	if err != nil {
		return err
	}
	vat := int64(math.Round(float64(subtotal) * float64(rate) / 100))
	total := subtotal + vat
	now := time.Now().Format(time.RFC3339)

	var invoiceID int64
	if isNew {
		number, err := nextInvoiceNumber(tx)
		if err != nil {
			return err
		}

		q := `INSERT INTO repair_invoice
		      (repair_id, number, subtotal_cents, vat_cents,
		       total_cents, created_at, updated_at)
		      VALUES (?, ?, ?, ?, ?, ?, ?)`
		res, err := tx.Exec(
			q, rid, number, subtotal, vat, total, now, now,
		)
		if err != nil {
			return err
		}

		invoiceID, err = res.LastInsertId()
		if err != nil {
			return err
		}
	} else {
		invoiceID = existing
		q := `UPDATE repair_invoice
		      SET subtotal_cents = ?, vat_cents = ?,
		          total_cents = ?, updated_at = ?
		      WHERE id = ?`
		if _, err := tx.Exec(q, subtotal, vat, total, now, invoiceID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		`DELETE FROM repair_invoice_lines WHERE invoice_id = ?`,
		invoiceID,
	); err != nil {
		return err
	}

	q := `INSERT INTO repair_invoice_lines
	      (invoice_id, work_type_id, kind, name, quantity, net_cents,
	       sort_order)
	      VALUES (?, NULLIF(?, 0), ?, ?, ?, ?, ?)`
	for i, line := range lines {
		if _, err := tx.Exec(
			q, invoiceID, line.WorkTypeID, line.Kind, line.Name,
			line.Quantity, line.NetCents, i+1,
		); err != nil {
			return err
		}
	}

	return nil
}

func getInvoiceID(tx *sql.Tx, rid int64) (int64, error) {
	var id int64
	q := `SELECT id FROM repair_invoice WHERE repair_id = ?`
	err := tx.QueryRow(q, rid).Scan(&id)
	return id, err
}

func nextInvoiceNumber(tx *sql.Tx) (string, error) {
	if err := ensureInvoiceSettingsTx(tx); err != nil {
		return "", err
	}

	q := `SELECT prefix, next_number, padding
	      FROM invoice_settings WHERE id = 1`
	var prefix string
	var next int64
	var padding int
	if err := tx.QueryRow(q).Scan(&prefix, &next, &padding); err != nil {
		return "", err
	}

	number := InvoiceNumber(prefix, next, padding)
	for {
		taken, err := invoiceNumberTakenTx(tx, number)
		if err != nil {
			return "", err
		}
		if !taken {
			break
		}
		next++
		number = InvoiceNumber(prefix, next, padding)
	}

	u := `UPDATE invoice_settings
	      SET next_number = ?, updated_at = ?
	      WHERE id = 1`
	_, err := tx.Exec(u, next+1, time.Now().Format(time.RFC3339))
	return number, err
}

func vatRateTx(tx *sql.Tx) (int, error) {
	if err := ensureInvoiceSettingsTx(tx); err != nil {
		return 0, err
	}

	var rate int
	q := `SELECT vat_rate FROM invoice_settings WHERE id = 1`
	if err := tx.QueryRow(q).Scan(&rate); err != nil {
		return 0, err
	}
	if rate < 0 {
		return DefaultVATRate, nil
	}
	return rate, nil
}

func InvoiceNumber(prefix string, next int64, padding int) string {
	width := strconv.Itoa(padding)
	return fmt.Sprintf("%s%s%0"+width+"d", prefix, invoiceSeparator(prefix), next)
}

func invoiceSeparator(prefix string) string {
	if prefix == "" {
		return ""
	}
	last := prefix[len(prefix)-1]
	switch last {
	case '-', '/', ' ', '_':
		return ""
	default:
		return "-"
	}
}

func invoiceNumberTaken(db *sql.DB, number string) (bool, error) {
	var exists int
	q := `SELECT 1 FROM repair_invoice WHERE number = ? LIMIT 1`
	err := db.QueryRow(q, number).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func nextFreeInvoiceNumber(
	db *sql.DB, prefix string, next int64, padding int,
) (int64, error) {
	for {
		taken, err := invoiceNumberTaken(
			db, InvoiceNumber(prefix, next, padding),
		)
		if err != nil {
			return 0, err
		}
		if !taken {
			return next, nil
		}
		next++
	}
}

func invoiceNumberTakenTx(tx *sql.Tx, number string) (bool, error) {
	var exists int
	q := `SELECT 1 FROM repair_invoice WHERE number = ? LIMIT 1`
	err := tx.QueryRow(q, number).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func ensureInvoiceSettings(db *sql.DB) error {
	now := time.Now().Format(time.RFC3339)
	q := `INSERT OR IGNORE INTO invoice_settings
	      (id, prefix, next_number, padding, vat_rate,
	       created_at, updated_at)
	      VALUES (1, '', 1, 4, ?, ?, ?)`
	_, err := db.Exec(q, DefaultVATRate, now, now)
	return err
}

func ensureInvoiceSettingsTx(tx *sql.Tx) error {
	now := time.Now().Format(time.RFC3339)
	q := `INSERT OR IGNORE INTO invoice_settings
	      (id, prefix, next_number, padding, vat_rate,
	       created_at, updated_at)
	      VALUES (1, '', 1, 4, ?, ?, ?)`
	_, err := tx.Exec(q, DefaultVATRate, now, now)
	return err
}

func loadLines(db *sql.DB, invoiceID int64) ([]InvoiceLine, error) {
	q := `SELECT id, invoice_id, COALESCE(work_type_id, 0), kind, name, quantity,
	             net_cents, sort_order
	      FROM repair_invoice_lines
	      WHERE invoice_id = ?
	      ORDER BY sort_order, id`

	rows, err := db.Query(q, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []InvoiceLine
	for rows.Next() {
		var line InvoiceLine
		err := rows.Scan(
			&line.ID, &line.InvoiceID, &line.WorkTypeID, &line.Kind,
			&line.Name, &line.Quantity, &line.NetCents,
			&line.SortOrder,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, line)
	}

	return list, rows.Err()
}

func sumLines(lines []InvoiceLineInput) int64 {
	var total int64
	for _, line := range lines {
		total += line.TotalCents()
	}
	return total
}

func (l InvoiceLineInput) TotalCents() int64 {
	return multiplyCents(l.NetCents, l.Quantity)
}

func Money(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}

	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func ParseMoney(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, nil
	}

	parts := strings.SplitN(s, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	var cents int64
	if len(parts) == 2 {
		dec := parts[1]
		if len(dec) > 2 {
			dec = dec[:2]
		}
		for len(dec) < 2 {
			dec += "0"
		}
		cents, err = strconv.ParseInt(dec, 10, 64)
		if err != nil {
			return 0, err
		}
	}

	if whole < 0 {
		return whole*100 - cents, nil
	}

	return whole*100 + cents, nil
}

func (i Invoice) Subtotal() string {
	return Money(i.SubtotalCents)
}

func (i Invoice) VAT() string {
	return Money(i.VATCents)
}

func (i Invoice) Total() string {
	return Money(i.TotalCents)
}

func (i Invoice) PrintNumber() string {
	pos := len(i.Number)
	for pos > 0 {
		ch := i.Number[pos-1]
		if ch < '0' || ch > '9' {
			break
		}
		pos--
	}
	if pos == len(i.Number) {
		return i.Number
	}
	return i.Number[pos:]
}

func (l InvoiceLine) Net() string {
	return Money(l.NetCents)
}

func (l InvoiceLine) TotalCents() int64 {
	return multiplyCents(l.NetCents, l.Quantity)
}

func (l InvoiceLine) TotalEuro() string {
	return euro(l.TotalCents())
}

func (l InvoiceLine) TotalCent() string {
	return cent(l.TotalCents())
}

func (i Invoice) SubtotalEuro() string {
	return euro(i.SubtotalCents)
}

func (i Invoice) SubtotalCent() string {
	return cent(i.SubtotalCents)
}

func (i Invoice) VATEuro() string {
	return euro(i.VATCents)
}

func (i Invoice) VATCent() string {
	return cent(i.VATCents)
}

func (i Invoice) TotalEuro() string {
	return euro(i.TotalCents)
}

func (i Invoice) TotalCent() string {
	return cent(i.TotalCents)
}

func (l InvoiceLine) Euro() string {
	return euro(l.TotalCents())
}

func (l InvoiceLine) Cent() string {
	return cent(l.TotalCents())
}

func multiplyCents(cents int64, quantity string) int64 {
	qty, err := strconv.ParseFloat(
		strings.ReplaceAll(strings.TrimSpace(quantity), ",", "."),
		64,
	)
	if err != nil || qty <= 0 {
		qty = 1
	}
	return int64(math.Round(float64(cents) * qty))
}

func euro(cents int64) string {
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%d", cents/100)
}

func cent(cents int64) string {
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%02d", cents%100)
}
