package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/imega/mygarage/appdir"
)

type Uploader interface {
	Upload(
		ctx context.Context, folderID, path, name string, keep int,
	) (string, error)
}

// FinalTimeout bounds a backup that runs without a waiting HTTP request.
const FinalTimeout = 2 * time.Minute

var mu sync.Mutex

// Create writes a consistent database snapshot to every enabled destination.
// Without a local destination the snapshot lives in a temporary directory
// only for the duration of the upload.
func Create(ctx context.Context, db *sql.DB, drive Uploader) error {
	mu.Lock()
	defer mu.Unlock()

	settings, err := GetSettings(db)
	if err != nil {
		return err
	}
	if !settings.Local && !settings.Drive {
		return nil
	}

	dir, err := snapshotDir(settings.Local)
	if err != nil {
		return err
	}
	if !settings.Local {
		defer os.RemoveAll(dir)
	}

	name := fmt.Sprintf("garage-%s.db", time.Now().Format("2006-01-02-150405"))
	path := filepath.Join(dir, name)
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("snapshot database: %w", err)
	}

	var errs []error
	if settings.Local {
		if err := Rotate(); err != nil {
			errs = append(errs, err)
		}
	}
	if settings.Drive {
		folderID, uploadErr := drive.Upload(
			ctx, settings.DriveFolderID, path, name, keep,
		)
		if uploadErr != nil {
			errs = append(errs, fmt.Errorf("Google Drive: %w", uploadErr))
		}
		if err := saveDriveResult(db, folderID, uploadErr); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func snapshotDir(local bool) (string, error) {
	if local {
		return appdir.Backups()
	}
	return os.MkdirTemp("", "mygarage-backup-")
}
