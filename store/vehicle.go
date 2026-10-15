package store

import (
	"database/sql"
	"time"
)

type Vehicle struct {
	ID          int64
	PlateNumber string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func FindVehicle(db *sql.DB, plate string) (*Vehicle, error) {
	q := `SELECT id, plate_number, created_at, updated_at
	      FROM vehicles WHERE plate_number = ?`

	var v Vehicle
	var created, updated string

	err := db.QueryRow(q, plate).Scan(
		&v.ID, &v.PlateNumber, &created, &updated,
	)
	if err != nil {
		return nil, err
	}

	v.CreatedAt, _ = time.Parse(time.RFC3339, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339, updated)

	return &v, nil
}

func FindVehicleByCompactPlate(
	db *sql.DB, plate string,
) (*Vehicle, error) {
	q := `SELECT id, plate_number, created_at, updated_at
	      FROM vehicles
	      WHERE REPLACE(REPLACE(UPPER(plate_number), ' ', ''), '-', '') = ?
	      LIMIT 1`

	var v Vehicle
	var created, updated string
	err := db.QueryRow(q, plate).Scan(
		&v.ID, &v.PlateNumber, &created, &updated,
	)
	if err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &v, nil
}

func GetVehicle(db *sql.DB, id int64) (*Vehicle, error) {
	q := `SELECT id, plate_number, created_at, updated_at
	      FROM vehicles WHERE id = ?`

	var v Vehicle
	var created, updated string

	err := db.QueryRow(q, id).Scan(
		&v.ID, &v.PlateNumber, &created, &updated,
	)
	if err != nil {
		return nil, err
	}

	v.CreatedAt, _ = time.Parse(time.RFC3339, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339, updated)

	return &v, nil
}

func CreateVehicle(db *sql.DB, plate string) (int64, error) {
	now := time.Now().Format(time.RFC3339)
	q := `INSERT INTO vehicles (plate_number, created_at, updated_at)
	      VALUES (?, ?, ?)`

	res, err := db.Exec(q, plate, now, now)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}
