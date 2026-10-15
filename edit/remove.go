package edit

import (
	"database/sql"

	"github.com/imega/mygarage/store"
)

func Remove(db *sql.DB, id int64) error {
	return store.DeleteRepair(db, id)
}
