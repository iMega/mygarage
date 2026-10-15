package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplyMigrations_DuplicatePlates_MergesOnce(t *testing.T) {
	database, err := sql.Open(
		"sqlite", filepath.Join(t.TempDir(), "garage.db"),
	)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	q := `
	CREATE TABLE vehicles (
	    id INTEGER PRIMARY KEY,
	    plate_number TEXT NOT NULL UNIQUE
	);
	CREATE TABLE repairs (
	    id INTEGER PRIMARY KEY,
	    vehicle_id INTEGER NOT NULL
	);
	CREATE TABLE schema_migrations (
	    name TEXT PRIMARY KEY,
	    applied_at TEXT NOT NULL
	);
	INSERT INTO vehicles (id, plate_number) VALUES (4, 'LK53 WUO');
	INSERT INTO vehicles (id, plate_number) VALUES (9, 'LK53WUO');
	INSERT INTO repairs (id, vehicle_id) VALUES (10, 4);
	INSERT INTO repairs (id, vehicle_id) VALUES (11, 9);`
	if _, err := database.Exec(q); err != nil {
		t.Fatalf("prepare database: %v", err)
	}

	if err := applyMigrations(database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM vehicles`,
	).Scan(&count); err != nil {
		t.Fatalf("count vehicles: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one vehicle, got %d", count)
	}

	var plate string
	if err := database.QueryRow(
		`SELECT plate_number FROM vehicles WHERE id = 4`,
	).Scan(&plate); err != nil {
		t.Fatalf("load vehicle: %v", err)
	}
	if plate != "LK53WUO" {
		t.Fatalf("expected compact plate, got %s", plate)
	}

	if err := database.QueryRow(
		`SELECT COUNT(*) FROM repairs WHERE vehicle_id = 4`,
	).Scan(&count); err != nil {
		t.Fatalf("count repairs: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two reassigned repairs, got %d", count)
	}

	if err := database.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`,
		normalizeVehiclePlatesMigration,
	).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected migration record, got %d", count)
	}

	if _, err := database.Exec(
		`UPDATE vehicles SET plate_number = 'LK53 WUO' WHERE id = 4`,
	); err != nil {
		t.Fatalf("change normalized plate: %v", err)
	}
	if err := applyMigrations(database); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	if err := database.QueryRow(
		`SELECT plate_number FROM vehicles WHERE id = 4`,
	).Scan(&plate); err != nil {
		t.Fatalf("reload vehicle: %v", err)
	}
	if plate != "LK53 WUO" {
		t.Fatalf("migration should not run twice, got %s", plate)
	}
}

func TestApplyMigration_Failure_DoesNotMarkApplied(t *testing.T) {
	database, err := sql.Open(
		"sqlite", filepath.Join(t.TempDir(), "failed.db"),
	)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(
		`CREATE TABLE schema_migrations (
		 name TEXT PRIMARY KEY,
		 applied_at TEXT NOT NULL
		)`,
	); err != nil {
		t.Fatalf("create migration table: %v", err)
	}

	item := migration{
		name: "failed_migration",
		up: func(*sql.Tx) error {
			return errors.New("failed")
		},
	}
	if err := applyMigration(database, item); err == nil {
		t.Fatal("expected migration failure")
	}

	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations`,
	).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed migration must not be recorded, got %d", count)
	}
}
