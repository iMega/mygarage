package backup

import (
	"database/sql"
	"time"
)

type Settings struct {
	Local           bool
	Drive           bool
	DriveFolderID   string
	DriveLastUpload string
	DriveLastError  string
}

func GetSettings(db *sql.DB) (*Settings, error) {
	if err := ensureSettings(db); err != nil {
		return nil, err
	}

	var local, drive int
	var settings Settings
	err := db.QueryRow(
		`SELECT local_enabled, drive_enabled, drive_folder_id,
		        drive_last_upload, drive_last_error
		 FROM backup_settings WHERE id = 1`,
	).Scan(
		&local, &drive, &settings.DriveFolderID,
		&settings.DriveLastUpload, &settings.DriveLastError,
	)
	if err != nil {
		return nil, err
	}
	settings.Local = local == 1
	settings.Drive = drive == 1

	return &settings, nil
}

func SaveSettings(db *sql.DB, local, drive bool) error {
	if err := ensureSettings(db); err != nil {
		return err
	}
	_, err := db.Exec(
		`UPDATE backup_settings
		 SET local_enabled = ?, drive_enabled = ?, updated_at = ?
		 WHERE id = 1`,
		flag(local), flag(drive), time.Now().Format(time.RFC3339),
	)
	return err
}

func saveDriveResult(db *sql.DB, folderID string, uploadErr error) error {
	now := time.Now().Format(time.RFC3339)
	if uploadErr != nil {
		_, err := db.Exec(
			`UPDATE backup_settings
			 SET drive_folder_id = ?, drive_last_error = ?, updated_at = ?
			 WHERE id = 1`,
			folderID, uploadErr.Error(), now,
		)
		return err
	}
	_, err := db.Exec(
		`UPDATE backup_settings
		 SET drive_folder_id = ?, drive_last_upload = ?,
		     drive_last_error = '', updated_at = ?
		 WHERE id = 1`,
		folderID, now, now,
	)
	return err
}

func ensureSettings(db *sql.DB) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT OR IGNORE INTO backup_settings
		 (id, local_enabled, drive_enabled, created_at, updated_at)
		 VALUES (1, 1, 0, ?, ?)`,
		now, now,
	)
	return err
}

func flag(value bool) int {
	if value {
		return 1
	}
	return 0
}
