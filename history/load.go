package history

import (
	"database/sql"

	"github.com/imega/mygarage/store"
)

type Data struct {
	Vehicle *store.Vehicle
	Repairs []Repair
	Summary Summary
}

type Repair struct {
	store.Repair
	HasInvoice bool
}

type Summary struct {
	Count       int
	LastDate    string
	LastMileage string
	LastStatus  string
}

func Load(db *sql.DB, id int64) (*Data, error) {
	q := `SELECT id, plate_number FROM vehicles WHERE id = ?`
	var v store.Vehicle
	if err := db.QueryRow(q, id).Scan(&v.ID, &v.PlateNumber); err != nil {
		return nil, err
	}

	repairs, err := store.RepairsByVehicle(db, id)
	if err != nil {
		return nil, err
	}

	rows, err := repairRows(db, repairs)
	if err != nil {
		return nil, err
	}

	return &Data{
		Vehicle: &v,
		Repairs: rows,
		Summary: summary(rows),
	}, nil
}

func repairRows(db *sql.DB, repairs []store.Repair) ([]Repair, error) {
	rows := make([]Repair, 0, len(repairs))
	for _, repair := range repairs {
		has := false
		invoice, err := store.GetInvoice(db, repair.ID)
		if err == nil {
			has = len(invoice.Lines) > 0
		} else if err != sql.ErrNoRows {
			return nil, err
		}

		rows = append(rows, Repair{
			Repair:     repair,
			HasInvoice: has,
		})
	}

	return rows, nil
}

func summary(repairs []Repair) Summary {
	s := Summary{Count: len(repairs)}
	if len(repairs) == 0 {
		return s
	}

	last := repairs[0]
	s.LastDate = last.RepairDate.Format("02.01.2006")
	s.LastMileage = last.Mileage
	s.LastStatus = last.StatusLabel()

	return s
}
