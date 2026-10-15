package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/imega/mygarage/db"
)

type fakeUploader struct {
	folderID string
	content  []byte
	err      error
}

func (f *fakeUploader) Upload(
	_ context.Context, folderID, path, _ string, _ int,
) (string, error) {
	f.folderID = folderID
	data, err := os.ReadFile(path)
	if err != nil {
		return folderID, err
	}
	f.content = data
	if f.err != nil {
		return folderID, f.err
	}
	return "folder", nil
}

func TestCreate_DriveOnly_UploadsSnapshotAndSavesFolder(t *testing.T) {
	database := openTestDB(t)
	if err := SaveSettings(database, false, true); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	uploader := &fakeUploader{}

	err := Create(context.Background(), database, uploader)

	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(uploader.content) == 0 {
		t.Fatal("expected non-empty snapshot")
	}
	settings, err := GetSettings(database)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if settings.DriveFolderID != "folder" {
		t.Fatalf("expected folder saved, got %q", settings.DriveFolderID)
	}
	if settings.DriveLastUpload == "" || settings.DriveLastError != "" {
		t.Fatalf("unexpected upload status %+v", settings)
	}
}

func TestCreate_DriveFails_KeepsFolderAndStoresError(t *testing.T) {
	database := openTestDB(t)
	if err := SaveSettings(database, false, true); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := saveDriveResult(database, "known", nil); err != nil {
		t.Fatalf("save drive result: %v", err)
	}
	uploader := &fakeUploader{err: errors.New("offline")}

	err := Create(context.Background(), database, uploader)

	if err == nil {
		t.Fatal("expected upload error")
	}
	settings, err := GetSettings(database)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if settings.DriveFolderID != "known" {
		t.Fatalf("expected folder kept, got %q", settings.DriveFolderID)
	}
	if settings.DriveLastError != "offline" {
		t.Fatalf("expected stored error, got %q", settings.DriveLastError)
	}
}

func TestCreate_AllDisabled_DoesNothing(t *testing.T) {
	database := openTestDB(t)
	if err := SaveSettings(database, false, false); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	uploader := &fakeUploader{}

	err := Create(context.Background(), database, uploader)

	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if uploader.content != nil {
		t.Fatal("expected no upload")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "garage.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.Exec(
		`CREATE TABLE backup_settings (
		    id INTEGER PRIMARY KEY CHECK (id = 1),
		    local_enabled INTEGER NOT NULL DEFAULT 1,
		    drive_enabled INTEGER NOT NULL DEFAULT 0,
		    drive_folder_id TEXT NOT NULL DEFAULT '',
		    drive_last_upload TEXT NOT NULL DEFAULT '',
		    drive_last_error TEXT NOT NULL DEFAULT '',
		    created_at TEXT NOT NULL,
		    updated_at TEXT NOT NULL
		)`,
	); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return database
}
