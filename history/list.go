package history

import (
	"database/sql"
	"time"

	"github.com/imega/mygarage/store"
)

type ListData struct {
	Repairs  []ListRepair
	Page     int
	Pages    int
	Total    int
	PrevPage int
	NextPage int
	HasPrev  bool
	HasNext  bool
}

type ListRepair struct {
	store.Repair
	InvoiceNumber string
	InvoiceTotal  string
	HasInvoice    bool
}

func LoadList(db *sql.DB, page, limit int) (*ListData, error) {
	if limit < 1 {
		limit = 1
	}

	var total int
	err := db.QueryRow(`SELECT COUNT(*) FROM repairs`).Scan(&total)
	if err != nil {
		return nil, err
	}

	pages := (total + limit - 1) / limit
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}

	repairs, err := listRepairs(db, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}

	return &ListData{
		Repairs:  repairs,
		Page:     page,
		Pages:    pages,
		Total:    total,
		PrevPage: page - 1,
		NextPage: page + 1,
		HasPrev:  page > 1,
		HasNext:  page < pages,
	}, nil
}

func listRepairs(db *sql.DB, limit, offset int) ([]ListRepair, error) {
	q := `SELECT r.id, r.vehicle_id, r.repair_date, r.mileage,
	             r.status, r.comment, r.created_at, r.updated_at,
	             v.plate_number, COALESCE(i.number, ''),
	             COALESCE(i.total_cents, 0),
	             CASE WHEN i.id IS NULL THEN 0 ELSE 1 END
	      FROM repairs r
	      JOIN vehicles v ON v.id = r.vehicle_id
	      LEFT JOIN repair_invoice i ON i.repair_id = r.id
	      ORDER BY r.repair_date DESC, r.id DESC
	      LIMIT ? OFFSET ?`

	rows, err := db.Query(q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]ListRepair, 0, limit)
	for rows.Next() {
		var row ListRepair
		var date, created, updated string
		var comment sql.NullString
		var total int64

		err := rows.Scan(
			&row.ID, &row.VehicleID, &date, &row.Mileage,
			&row.Status, &comment, &created, &updated,
			&row.PlateNumber, &row.InvoiceNumber, &total,
			&row.HasInvoice,
		)
		if err != nil {
			return nil, err
		}

		row.RepairDate, _ = time.Parse("2006-01-02", date)
		row.CreatedAt, _ = time.Parse(time.RFC3339, created)
		row.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		row.Comment = comment.String
		row.InvoiceTotal = store.Money(total)
		list = append(list, row)
	}

	return list, rows.Err()
}
