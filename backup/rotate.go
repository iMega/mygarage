package backup

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/imega/mygarage/appdir"
)

const keep = 20

func Rotate() error {
	dir, err := appdir.Backups()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}

	if len(files) <= keep {
		return nil
	}

	sort.Strings(files)

	for i := 0; i < len(files)-keep; i++ {
		path := filepath.Join(dir, files[i])
		if err := os.Remove(path); err != nil {
			return err
		}
	}

	return nil
}
