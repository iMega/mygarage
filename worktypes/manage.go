package worktypes

import (
	"database/sql"

	"github.com/imega/mygarage/store"
)

type Data struct {
	Category          string
	Name              string
	InvoiceNameEN     string
	DefaultPriceCents int64
	SortOrder         int
}

func Create(db *sql.DB, data Data) (int64, error) {
	return store.CreateWorkType(
		db, data.Category, data.Name, data.InvoiceNameEN,
		data.DefaultPriceCents, data.SortOrder,
	)
}

func Update(db *sql.DB, id int64, data Data) error {
	return store.UpdateWorkType(
		db, id, data.Category, data.Name, data.InvoiceNameEN,
		data.DefaultPriceCents, data.SortOrder,
	)
}

func Disable(db *sql.DB, id int64) error {
	return store.DisableWorkType(db, id)
}

func Enable(db *sql.DB, id int64) error {
	return store.EnableWorkType(db, id)
}

func List(db *sql.DB, show string) ([]store.WorkType, error) {
	return store.WorkTypes(db, show)
}

func SortOrder(db *sql.DB, id int64) (int, error) {
	var order int
	err := db.QueryRow(`
		SELECT sort_order FROM work_types WHERE id = ?
	`, id).Scan(&order)
	return order, err
}

func NextSortOrder(db *sql.DB) (int, error) {
	var maxOrder int
	err := db.QueryRow(`
		SELECT COALESCE(MAX(sort_order), 0) FROM work_types
	`).Scan(&maxOrder)
	if err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

func MoveUp(db *sql.DB, id int64) error {
	var currentOrder int
	err := db.QueryRow(`
		SELECT sort_order FROM work_types WHERE id = ?
	`, id).Scan(&currentOrder)
	if err != nil {
		return err
	}

	var prevID int64
	var prevOrder int
	err = db.QueryRow(`
		SELECT id, sort_order
		FROM work_types
		WHERE sort_order < ? AND is_active = 1
		ORDER BY sort_order DESC
		LIMIT 1
	`, currentOrder).Scan(&prevID, &prevOrder)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE work_types SET sort_order = ? WHERE id = ?
	`, prevOrder, id)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE work_types SET sort_order = ? WHERE id = ?
	`, currentOrder, prevID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
