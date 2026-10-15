package calendar

import (
	"database/sql"
	"strings"
	"time"
)

type Settings struct {
	Enabled    bool
	CalendarID string
}

func GetSettings(db *sql.DB) (*Settings, error) {
	if err := ensureSettings(db); err != nil {
		return nil, err
	}

	var enabled int
	var settings Settings
	err := db.QueryRow(
		`SELECT enabled, calendar_id FROM calendar_settings WHERE id = 1`,
	).Scan(&enabled, &settings.CalendarID)
	if err != nil {
		return nil, err
	}
	settings.Enabled = enabled == 1

	return &settings, nil
}

func SaveSettings(db *sql.DB, enabled bool, calendarID string) error {
	now := time.Now().Format(time.RFC3339)
	value := 0
	if enabled {
		value = 1
	}

	q := `INSERT INTO calendar_settings
	      (id, enabled, calendar_id, created_at, updated_at)
	      VALUES (1, ?, ?, ?, ?)
	      ON CONFLICT(id) DO UPDATE SET
	          enabled = excluded.enabled,
	          calendar_id = excluded.calendar_id,
	          updated_at = excluded.updated_at`
	_, err := db.Exec(q, value, strings.TrimSpace(calendarID), now, now)
	return err
}

func ensureSettings(db *sql.DB) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT OR IGNORE INTO calendar_settings
		 (id, enabled, calendar_id, created_at, updated_at)
		 VALUES (1, 0, '', ?, ?)`,
		now, now,
	)
	return err
}
