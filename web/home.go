package web

import (
	"context"
	"database/sql"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	garagecalendar "github.com/imega/mygarage/calendar"
	"github.com/imega/mygarage/store"
)

type HomeData struct {
	Queue         []QueueRepair
	Appointments  []garagecalendar.Visit
	CalendarError string
	Summary       QueueSummary
}

type QueueRepair struct {
	Repair     *store.Repair
	Total      string
	TotalCents int64
	HasInvoice bool
	CanPay     bool
}

type QueueSummary struct {
	Count      int
	Total      string
	Ready      int
	InProgress int
	Draft      int
}

func Home(
	db *sql.DB, tmpl *template.Template, agenda *garagecalendar.Agenda,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := loadHome(db, agenda)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "index", data)
	}
}

func loadHome(
	db *sql.DB, agenda *garagecalendar.Agenda,
) (*HomeData, error) {
	repairs, err := store.AllRepairs(db)
	if err != nil {
		return nil, err
	}

	var rows []QueueRepair
	var total int64
	var ready, progress, draft int
	for i := range repairs {
		repair := &repairs[i]
		if repair.Status == "paid" {
			continue
		}
		if strings.TrimSpace(repair.PlateNumber) == "" {
			continue
		}

		var cents int64
		var hasInvoice bool
		invoice, err := store.GetInvoice(db, repair.ID)
		if err == nil {
			cents = invoice.TotalCents
			hasInvoice = len(invoice.Lines) > 0
		} else if err != sql.ErrNoRows {
			return nil, err
		}

		total += cents
		switch repair.Status {
		case "draft":
			draft++
		case "in_progress":
			progress++
		default:
			ready++
		}

		rows = append(rows, QueueRepair{
			Repair:     repair,
			Total:      store.Money(cents),
			TotalCents: cents,
			HasInvoice: hasInvoice,
			CanPay:     cents > 0,
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		a := statusRank(rows[i].Repair.Status)
		b := statusRank(rows[j].Repair.Status)
		if a != b {
			return a < b
		}
		if rows[i].CanPay != rows[j].CanPay {
			return rows[i].CanPay
		}
		return rows[i].Repair.RepairDate.After(
			rows[j].Repair.RepairDate,
		)
	})

	data := &HomeData{
		Queue: rows,
		Summary: QueueSummary{
			Count:      len(rows),
			Total:      store.Money(total),
			Ready:      ready,
			InProgress: progress,
			Draft:      draft,
		},
	}
	if agenda != nil {
		appointments, err := agenda.Today(context.Background())
		if err != nil {
			data.CalendarError = err.Error()
		} else {
			data.Appointments = appointments
		}
	}

	return data, nil
}

func statusRank(status string) int {
	switch status {
	case "ready":
		return 0
	case "in_progress":
		return 1
	case "draft":
		return 2
	default:
		return 3
	}
}
