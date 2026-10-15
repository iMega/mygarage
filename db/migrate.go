package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/imega/mygarage/appdir"
	"github.com/imega/mygarage/normalize"
)

const schema = `
CREATE TABLE IF NOT EXISTS vehicles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plate_number TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS work_types (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    invoice_name_en TEXT NOT NULL DEFAULT '',
    default_price_cents INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS repairs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vehicle_id INTEGER NOT NULL,
    repair_date TEXT NOT NULL,
    mileage TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'in_progress',
    comment TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (vehicle_id) REFERENCES vehicles(id)
);

CREATE INDEX IF NOT EXISTS repairs_history_idx
    ON repairs(repair_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS repair_work_types (
    repair_id INTEGER NOT NULL,
    work_type_id INTEGER NOT NULL,
    PRIMARY KEY (repair_id, work_type_id),
    FOREIGN KEY (repair_id) REFERENCES repairs(id)
        ON DELETE CASCADE,
    FOREIGN KEY (work_type_id) REFERENCES work_types(id)
);

CREATE TABLE IF NOT EXISTS repair_invoice (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repair_id INTEGER NOT NULL UNIQUE,
    number TEXT NOT NULL UNIQUE,
    subtotal_cents INTEGER NOT NULL DEFAULT 0,
    vat_cents INTEGER NOT NULL DEFAULT 0,
    total_cents INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (repair_id) REFERENCES repairs(id)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS repair_invoice_lines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    invoice_id INTEGER NOT NULL,
    work_type_id INTEGER,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    quantity TEXT NOT NULL,
    net_cents INTEGER NOT NULL,
    sort_order INTEGER NOT NULL,
    FOREIGN KEY (work_type_id) REFERENCES work_types(id),
    FOREIGN KEY (invoice_id) REFERENCES repair_invoice(id)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS invoice_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    prefix TEXT NOT NULL DEFAULT '',
    next_number INTEGER NOT NULL DEFAULT 1,
    padding INTEGER NOT NULL DEFAULT 4,
    vat_rate INTEGER NOT NULL DEFAULT 19,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS company_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    name TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    vat_number TEXT NOT NULL DEFAULT '',
    fpa_number TEXT NOT NULL DEFAULT '',
    aft_number TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    technician_name TEXT NOT NULL DEFAULT '',
    technician_speciality TEXT NOT NULL DEFAULT '',
    technician_registry_number TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS calendar_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0,
    calendar_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS backup_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    local_enabled INTEGER NOT NULL DEFAULT 1,
    drive_enabled INTEGER NOT NULL DEFAULT 0,
    drive_folder_id TEXT NOT NULL DEFAULT '',
    drive_last_upload TEXT NOT NULL DEFAULT '',
    drive_last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    name TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);
`

const normalizeVehiclePlatesMigration = "20260712_normalize_vehicle_plates"

type migration struct {
	name string
	up   func(*sql.Tx) error
}

var migrations = []migration{
	{
		name: normalizeVehiclePlatesMigration,
		up:   normalizeVehicles,
	},
}

func Migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	if err := loadInit(db); err != nil {
		return err
	}
	if err := applyMigrations(db); err != nil {
		return err
	}

	return nil
}

type vehiclePlate struct {
	id    int64
	plate string
}

func applyMigrations(db *sql.DB) error {
	for _, item := range migrations {
		if err := applyMigration(db, item); err != nil {
			return fmt.Errorf("migration %s: %w", item.name, err)
		}
	}
	return nil
}

func applyMigration(db *sql.DB, item migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var applied int
	err = tx.QueryRow(
		`SELECT 1 FROM schema_migrations WHERE name = ?`, item.name,
	).Scan(&applied)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	if err := item.up(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
		item.name, time.Now().Format(time.RFC3339),
	); err != nil {
		return err
	}

	return tx.Commit()
}

func normalizeVehicles(tx *sql.Tx) error {
	rows, err := tx.Query(
		`SELECT id, plate_number FROM vehicles ORDER BY id`,
	)
	if err != nil {
		return err
	}

	var vehicles []vehiclePlate
	for rows.Next() {
		var vehicle vehiclePlate
		if err := rows.Scan(&vehicle.id, &vehicle.plate); err != nil {
			rows.Close()
			return err
		}
		vehicles = append(vehicles, vehicle)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	groups := make(map[string][]int64)
	for _, vehicle := range vehicles {
		plate := normalize.Plate(vehicle.plate)
		if plate == "" {
			continue
		}
		groups[plate] = append(groups[plate], vehicle.id)
	}

	for plate, ids := range groups {
		target := ids[0]
		for _, duplicate := range ids[1:] {
			if _, err := tx.Exec(
				`UPDATE repairs SET vehicle_id = ? WHERE vehicle_id = ?`,
				target, duplicate,
			); err != nil {
				return err
			}
			if _, err := tx.Exec(
				`DELETE FROM vehicles WHERE id = ?`, duplicate,
			); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(
			`UPDATE vehicles SET plate_number = ? WHERE id = ?`,
			plate, target,
		); err != nil {
			return err
		}
	}

	return nil
}

func loadInit(db *sql.DB) error {
	initFile, err := appdir.InitSQL()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(initFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if _, err := db.Exec(string(data)); err != nil {
		return err
	}

	// Rename init.sql to init.runned.YYYY-MM-DD-HH-MM-SS.sql
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	dir := filepath.Dir(initFile)
	newName := filepath.Join(dir, fmt.Sprintf("init.runned.%s.sql", timestamp))

	if err := os.Rename(initFile, newName); err != nil {
		return err
	}

	return nil
}
