package appdir

import (
	"os"
	"path/filepath"
)

const appName = "myGarage"

var rootOverride string

// SetRoot replaces the per-user data directory, e.g. /var/lib/mygarage on a
// server. Call it before any other function of this package.
func SetRoot(dir string) {
	rootOverride = dir
}

func Root() (string, error) {
	if rootOverride != "" {
		if err := os.MkdirAll(rootOverride, 0755); err != nil {
			return "", err
		}
		return rootOverride, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	appDir := filepath.Join(configDir, appName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	return appDir, nil
}

func Config() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

func DB() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "garage.db"), nil
}

func InitSQL() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "init.sql"), nil
}

func Backups() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}

	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", err
	}

	return backupDir, nil
}

func Assets() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}

	assetsDir := filepath.Join(root, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", err
	}

	return assetsDir, nil
}

func GoogleCalendarCredentials() (string, error) {
	return accessFile("google-calendar.json")
}

func GoogleDriveClient() (string, error) {
	return accessFile("google-drive-client.json")
}

func GoogleDriveToken() (string, error) {
	return accessFile("google-drive-token.json")
}

func accessFile(name string) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(root, "access")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(dir, name), nil
}
