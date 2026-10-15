package calendar

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/imega/mygarage/store"
)

type Visit struct {
	Plate       string
	Time        string
	Date        string
	VehicleID   int64
	Known       bool
	LastMileage string
	CreateURL   string
}

type Agenda struct {
	db     *sql.DB
	reader eventReader
	now    func() time.Time
}

type eventReader interface {
	read(
		ctx context.Context, calendarID string, from, to time.Time,
	) ([]event, error)
}

func New(db *sql.DB, credentialsPath string) *Agenda {
	return &Agenda{
		db:     db,
		reader: newGoogleReader(credentialsPath),
		now:    time.Now,
	}
}

func (a *Agenda) Today(ctx context.Context) ([]Visit, error) {
	settings, err := GetSettings(a.db)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, nil
	}
	if strings.TrimSpace(settings.CalendarID) == "" {
		return nil, fmt.Errorf("Google Calendar ID is not configured")
	}

	now := a.now()
	from := time.Date(
		now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location(),
	)
	events, err := a.reader.read(
		ctx, settings.CalendarID, from, from.AddDate(0, 0, 1),
	)
	if err != nil {
		return nil, err
	}

	visits := make([]Visit, 0, len(events))
	for _, item := range events {
		plate := plateFromTitle(item.Title)
		if plate == "" {
			continue
		}
		visit := Visit{
			Plate: plate,
			Time:  item.Start.In(now.Location()).Format("15:04"),
			Date:  from.Format("2006-01-02"),
		}
		if item.AllDay {
			visit.Time = "Весь день"
		}
		vehicle, err := store.FindVehicleByCompactPlate(
			a.db, plate,
		)
		if err == nil {
			visit.Known = true
			visit.VehicleID = vehicle.ID
			repairs, loadErr := store.RepairsByVehicle(a.db, vehicle.ID)
			if loadErr != nil {
				return nil, loadErr
			}
			if repairedOn(repairs, visit.Date) {
				continue
			}
			if len(repairs) > 0 {
				visit.LastMileage = repairs[0].Mileage
			}
		} else if err != sql.ErrNoRows {
			return nil, err
		}
		query := url.Values{}
		query.Set("plate", visit.Plate)
		query.Set("date", visit.Date)
		visit.CreateURL = "/repairs/new?" + query.Encode()
		visits = append(visits, visit)
	}

	return visits, nil
}

func repairedOn(repairs []store.Repair, date string) bool {
	for _, repair := range repairs {
		if repair.RepairDate.Format("2006-01-02") == date {
			return true
		}
	}
	return false
}
