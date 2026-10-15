package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// pragmas let several processes (blue/green slots) share one database file:
// WAL allows readers alongside a writer, busy_timeout waits for the lock
// instead of failing with SQLITE_BUSY.
const pragmas = "?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)"

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+pragmas)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}
