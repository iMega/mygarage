package search

import (
	"database/sql"

	"github.com/imega/mygarage/normalize"
	"github.com/imega/mygarage/store"
)

type Result struct {
	Vehicle     *store.Vehicle
	Repairs     []store.Repair
	Found       bool
	PlateNumber string
	WorkTypes   []store.WorkType
	Date        string
}

func Find(db *sql.DB, plate string) (*Result, error) {
	plate = normalize.Plate(plate)
	if plate == "" {
		return &Result{Found: false}, nil
	}

	v, err := store.FindVehicle(db, plate)
	if err == sql.ErrNoRows {
		return &Result{Found: false}, nil
	}
	if err != nil {
		return nil, err
	}

	repairs, err := store.RepairsByVehicle(db, v.ID)
	if err != nil {
		return nil, err
	}

	return &Result{
		Vehicle: v,
		Repairs: repairs,
		Found:   true,
	}, nil
}
