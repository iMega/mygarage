package store

import (
	"database/sql"
	"time"
)

type WorkType struct {
	ID                int64
	Category          string
	Name              string
	InvoiceNameEN     string
	DefaultPriceCents int64
	SortOrder         int
	IsActive          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func AllWorkTypes(db *sql.DB) ([]WorkType, error) {
	return WorkTypes(db, "categorized")
}

func WorkTypes(db *sql.DB, show string) ([]WorkType, error) {
	where := "WHERE is_active = 1"
	order := "ORDER BY sort_order, category, name"
	switch show {
	case "categorized":
		where = "WHERE is_active = 1 AND TRIM(category) <> ''"
		order = "ORDER BY category, sort_order, name"
	case "hidden":
		where = "WHERE is_active = 0"
	case "uncategorized":
		where = "WHERE is_active = 1 AND TRIM(category) = ''"
	case "all":
		where = ""
		order = "ORDER BY is_active DESC, sort_order, category, name"
	}

	q := `SELECT id, category, name, invoice_name_en,
	             default_price_cents, sort_order, is_active, created_at,
	             updated_at
	      FROM work_types ` + where + " " + order

	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []WorkType
	for rows.Next() {
		var w WorkType
		var created, updated string
		var active int

		err := rows.Scan(
			&w.ID, &w.Category, &w.Name, &w.InvoiceNameEN,
			&w.DefaultPriceCents, &w.SortOrder, &active, &created,
			&updated,
		)
		if err != nil {
			return nil, err
		}

		w.IsActive = active == 1
		w.CreatedAt, _ = time.Parse(time.RFC3339, created)
		w.UpdatedAt, _ = time.Parse(time.RFC3339, updated)

		list = append(list, w)
	}

	return list, rows.Err()
}

func CreateWorkType(
	db *sql.DB, category, name, invoiceName string,
	defaultPriceCents int64, order int,
) (int64, error) {
	now := time.Now().Format(time.RFC3339)
	q := `INSERT INTO work_types
	      (category, name, invoice_name_en, default_price_cents,
	       sort_order, is_active, created_at, updated_at)
	      VALUES (?, ?, ?, ?, ?, 1, ?, ?)`

	res, err := db.Exec(
		q, category, name, invoiceName, defaultPriceCents, order, now, now,
	)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func UpdateWorkType(
	db *sql.DB, id int64, category, name, invoiceName string,
	defaultPriceCents int64, order int,
) error {
	now := time.Now().Format(time.RFC3339)
	q := `UPDATE work_types
	      SET category = ?, name = ?, invoice_name_en = ?,
	          default_price_cents = ?, sort_order = ?, updated_at = ?
	      WHERE id = ?`

	_, err := db.Exec(
		q, category, name, invoiceName, defaultPriceCents, order, now, id,
	)
	return err
}

func DisableWorkType(db *sql.DB, id int64) error {
	now := time.Now().Format(time.RFC3339)
	q := `UPDATE work_types
	      SET is_active = 0, updated_at = ?
	      WHERE id = ?`

	_, err := db.Exec(q, now, id)
	return err
}

func EnableWorkType(db *sql.DB, id int64) error {
	now := time.Now().Format(time.RFC3339)
	q := `UPDATE work_types
	      SET is_active = 1, updated_at = ?
	      WHERE id = ?`

	_, err := db.Exec(q, now, id)
	return err
}

func (w WorkType) InvoiceName() string {
	return w.InvoiceNameEN
}

func (w WorkType) DefaultPrice() string {
	return Money(w.DefaultPriceCents)
}
