package store

import (
	"database/sql"
	"strings"
	"time"
)

type Repair struct {
	ID          int64
	VehicleID   int64
	PlateNumber string
	RepairDate  time.Time
	Mileage     string
	Status      string
	Comment     string
	WorkTypes   []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r Repair) StatusLabel() string {
	switch r.Status {
	case "draft":
		return "Черновик"
	case "in_progress":
		return "В работе"
	case "paid":
		return "Оплачено"
	default:
		return "Готово"
	}
}

func (r Repair) DisplayComment() string {
	comment := strings.TrimSpace(r.Comment)
	if comment == "" {
		return ""
	}
	if strings.EqualFold(comment, strings.TrimSpace(r.Mileage)) {
		return ""
	}
	if isMileageOnly(comment) {
		return ""
	}
	return comment
}

func normalizeStatus(status string) string {
	switch status {
	case "draft", "in_progress", "ready", "paid":
		return status
	default:
		return "in_progress"
	}
}

func normalizeMileage(mileage string) string {
	mileage = strings.TrimSpace(mileage)
	if mileage == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range mileage {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isMileageOnly(comment string) bool {
	value := strings.ToLower(strings.TrimSpace(comment))
	value = strings.ReplaceAll(value, " ", "")

	suffixes := []string{"km", "км", "ml", "mil", "mile", "miles"}
	for _, suffix := range suffixes {
		if !strings.HasSuffix(value, suffix) {
			continue
		}
		number := strings.TrimSuffix(value, suffix)
		return number != "" && onlyDigits(number)
	}

	return onlyDigits(value)
}

func onlyDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func RepairsByVehicle(db *sql.DB, vid int64) ([]Repair, error) {
	q := `SELECT id, vehicle_id, repair_date, mileage, status, comment,
	             created_at, updated_at
	      FROM repairs
	      WHERE vehicle_id = ?
	      ORDER BY repair_date DESC, id DESC`

	rows, err := db.Query(q, vid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Repair
	for rows.Next() {
		var r Repair
		var date, created, updated string
		var comment sql.NullString

		err := rows.Scan(
			&r.ID, &r.VehicleID, &date, &r.Mileage, &r.Status,
			&comment,
			&created, &updated,
		)
		if err != nil {
			return nil, err
		}

		r.RepairDate, _ = time.Parse("2006-01-02", date)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		r.Comment = comment.String

		r.WorkTypes, err = loadWorks(db, r.ID)
		if err != nil {
			return nil, err
		}

		list = append(list, r)
	}

	return list, rows.Err()
}

func GetRepair(db *sql.DB, id int64) (*Repair, error) {
	q := `SELECT id, vehicle_id, repair_date, mileage, status, comment,
	             created_at, updated_at
	      FROM repairs WHERE id = ?`

	var r Repair
	var date, created, updated string
	var comment sql.NullString

	err := db.QueryRow(q, id).Scan(
		&r.ID, &r.VehicleID, &date, &r.Mileage, &r.Status,
		&comment,
		&created, &updated,
	)
	if err != nil {
		return nil, err
	}

	r.RepairDate, _ = time.Parse("2006-01-02", date)
	r.CreatedAt, _ = time.Parse(time.RFC3339, created)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	r.Comment = comment.String

	r.WorkTypes, err = loadWorks(db, r.ID)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

func CreateRepair(
	db *sql.DB, vid int64, date, mileage, status, comment string,
	works []int64, lines []InvoiceLineInput,
) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	q := `INSERT INTO repairs
	      (vehicle_id, repair_date, mileage, status, comment,
	       created_at, updated_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?)`

	res, err := tx.Exec(
		q, vid, date, normalizeMileage(mileage), normalizeStatus(status),
		comment, now, now,
	)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := saveWorks(tx, id, works); err != nil {
		return 0, err
	}

	if err := saveInvoice(tx, id, lines); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return id, nil
}

func UpdateRepair(
	db *sql.DB, id, vid int64, date, mileage, status, comment string,
	works []int64, lines []InvoiceLineInput,
) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	if vid > 0 {
		q := `UPDATE repairs
		      SET vehicle_id = ?, repair_date = ?, mileage = ?,
		          status = ?, comment = ?, updated_at = ?
		      WHERE id = ?`

		if _, err := tx.Exec(
			q, vid, date, normalizeMileage(mileage),
			normalizeStatus(status), comment, now, id,
		); err != nil {
			return err
		}
	} else {
		q := `UPDATE repairs
		      SET repair_date = ?, mileage = ?, status = ?,
		          comment = ?, updated_at = ?
		      WHERE id = ?`

		if _, err := tx.Exec(
			q, date, normalizeMileage(mileage), normalizeStatus(status),
			comment, now, id,
		); err != nil {
			return err
		}
	}

	del := `DELETE FROM repair_work_types WHERE repair_id = ?`
	if _, err := tx.Exec(del, id); err != nil {
		return err
	}

	if err := saveWorks(tx, id, works); err != nil {
		return err
	}

	if err := saveInvoice(tx, id, lines); err != nil {
		return err
	}

	return tx.Commit()
}

func DeleteRepair(db *sql.DB, id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	delLines := `DELETE FROM repair_invoice_lines
	             WHERE invoice_id IN (
	                 SELECT id FROM repair_invoice WHERE repair_id = ?
	             )`
	if _, err := tx.Exec(delLines, id); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`DELETE FROM repair_invoice WHERE repair_id = ?`, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`DELETE FROM repair_work_types WHERE repair_id = ?`, id,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM repairs WHERE id = ?`, id); err != nil {
		return err
	}

	return tx.Commit()
}

func MarkRepairPaid(db *sql.DB, id int64) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(
		`UPDATE repairs SET status = ?, updated_at = ? WHERE id = ?`,
		"paid", now, id,
	)
	return err
}

func AllRepairs(db *sql.DB) ([]Repair, error) {
	q := `SELECT r.id, r.vehicle_id, r.repair_date, r.mileage,
	             r.status, r.comment,
	             r.created_at, r.updated_at, v.plate_number
	      FROM repairs r
	      JOIN vehicles v ON v.id = r.vehicle_id
	      ORDER BY r.repair_date DESC, r.id DESC`

	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Repair
	for rows.Next() {
		var r Repair
		var date, created, updated string
		var comment sql.NullString

		err := rows.Scan(
			&r.ID, &r.VehicleID, &date, &r.Mileage, &r.Status,
			&comment,
			&created, &updated, &r.PlateNumber,
		)
		if err != nil {
			return nil, err
		}

		r.RepairDate, _ = time.Parse("2006-01-02", date)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		r.Comment = comment.String

		r.WorkTypes, err = loadWorks(db, r.ID)
		if err != nil {
			return nil, err
		}

		list = append(list, r)
	}

	return list, rows.Err()
}

func loadWorks(db *sql.DB, rid int64) ([]string, error) {
	q := `SELECT wt.name
	      FROM repair_work_types rwt
	      JOIN work_types wt ON wt.id = rwt.work_type_id
	      WHERE rwt.repair_id = ?
	      ORDER BY wt.sort_order, wt.name`

	rows, err := db.Query(q, rid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		list = append(list, name)
	}

	return list, rows.Err()
}

func GetRepairWorkTypeIDs(db *sql.DB, rid int64) ([]int64, error) {
	q := `SELECT work_type_id
	      FROM repair_work_types
	      WHERE repair_id = ?`

	rows, err := db.Query(q, rid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		list = append(list, id)
	}

	return list, rows.Err()
}

type tx interface {
	Exec(string, ...interface{}) (sql.Result, error)
}

func saveWorks(tx tx, rid int64, works []int64) error {
	if len(works) == 0 {
		return nil
	}

	q := `INSERT INTO repair_work_types
	      (repair_id, work_type_id) VALUES (?, ?)`

	for _, wid := range works {
		if _, err := tx.Exec(q, rid, wid); err != nil {
			return err
		}
	}

	return nil
}
