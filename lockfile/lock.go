package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/imega/mygarage/appdir"
)

const lockFileName = "app.lock"

type Lock struct {
	path string
	file *os.File
}

func Acquire() (*Lock, error) {
	root, err := appdir.Root()
	if err != nil {
		return nil, err
	}

	lockPath := filepath.Join(root, lockFileName)

	file, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("another instance is running")
	}

	pid := os.Getpid()
	if err := file.Truncate(0); err != nil {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
		return nil, err
	}

	if _, err := file.WriteString(strconv.Itoa(pid)); err != nil {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
		return nil, err
	}

	return &Lock{path: lockPath, file: file}, nil
}

func (l *Lock) Release() error {
	if l.file == nil {
		return nil
	}

	syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	l.file.Close()
	os.Remove(l.path)

	return nil
}
