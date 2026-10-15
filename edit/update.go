package edit

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

func Update(db *sql.DB, id int64, data Data) error {
	works, err := parseWorks(data.Works)
	if err != nil {
		return err
	}

	vid, err := vehicleID(db, data.Plate)
	if err != nil {
		return err
	}

	return store.UpdateRepair(
		db, id, vid, data.Date, data.Mileage, data.Status,
		data.Comment, works, data.Lines,
	)
}

func vehicleID(db *sql.DB, raw string) (int64, error) {
	plate := normalize.Plate(raw)
	if plate == "" {
		return 0, nil
	}

	v, err := store.FindVehicle(db, plate)
	if err == nil {
		return v.ID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	return store.CreateVehicle(db, plate)
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

func ParseLines(
	workTypeIDs, names, quantities, amounts []string,
) ([]store.InvoiceLineInput, error) {
	var lines []store.InvoiceLineInput
	for i, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		cents, err := store.ParseMoney(at(amounts, i))
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
