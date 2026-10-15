package attach

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/imega/mygarage/normalize"
	"github.com/imega/mygarage/store"
)

const maxInvoiceLines = 14

type Data struct {
	Plate   string
	Date    string
	Mileage string
	Status  string
	Comment string
	Works   []string
	Lines   []store.InvoiceLineInput
}

type Result struct {
	VehicleID int64
	RepairID  int64
}

func Attach(db *sql.DB, data Data) (*Result, error) {
	plate := normalize.Plate(data.Plate)

	v, err := store.FindVehicle(db, plate)
	if err == sql.ErrNoRows {
		id, err := store.CreateVehicle(db, plate)
		if err != nil {
			return nil, err
		}
		v = &store.Vehicle{ID: id, PlateNumber: plate}
	} else if err != nil {
		return nil, err
	}

	works, err := parseWorks(data.Works)
	if err != nil {
		return nil, err
	}

	rid, err := store.CreateRepair(
		db, v.ID, data.Date, data.Mileage, data.Status,
		data.Comment, works, data.Lines,
	)
	if err != nil {
		return nil, err
	}

	return &Result{
		VehicleID: v.ID,
		RepairID:  rid,
	}, nil
}

func ParseLines(
	workTypeIDs, names, quantities, amounts []string,
) ([]store.InvoiceLineInput, error) {
	var lines []store.InvoiceLineInput
	for i, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		amount := at(amounts, i)
		cents, err := store.ParseMoney(amount)
		if err != nil {
			return nil, err
		}
		if cents == 0 {
			continue
		}
		if len(lines) >= maxInvoiceLines {
			break
		}

		workTypeID, _ := strconv.ParseInt(at(workTypeIDs, i), 10, 64)

		qty := strings.TrimSpace(at(quantities, i))
		if qty == "" {
			qty = "1"
		}

		lines = append(lines, store.InvoiceLineInput{
			WorkTypeID: workTypeID,
			Kind:       "work",
			Name:       name,
			Quantity:   qty,
			NetCents:   cents,
		})
	}

	return lines, nil
}

func at(list []string, i int) string {
	if i >= len(list) {
		return ""
	}
	return list[i]
}

func parseWorks(raw []string) ([]int64, error) {
	var ids []int64
	for _, s := range raw {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}
