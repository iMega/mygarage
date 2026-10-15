package attach

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/repairform"
	"github.com/imega/mygarage/store"
)

type handlers struct {
	db   *sql.DB
	tmpl *template.Template
}

func Handlers(db *sql.DB, tmpl *template.Template) *handlers {
	return &handlers{db: db, tmpl: tmpl}
}

func (h *handlers) ShowForm(c *gin.Context) {
	plate := c.Query("plate")
	date := c.Query("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		date = time.Now().Format("2006-01-02")
	}

	types, err := store.AllWorkTypes(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	settings, err := store.GetInvoiceSettings(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	data := map[string]interface{}{
		"Plate":       plate,
		"Date":        date,
		"Status":      "in_progress",
		"VATRate":     settings.VATRate,
		"WorkTypes":   types,
		"WorkGroups":  repairform.Groups(types),
		"SelectedIDs": map[int64]bool{},
	}

	c.HTML(http.StatusOK, "repair_form", data)
}

func (h *handlers) Create(c *gin.Context) {
	lines, err := ParseLines(
		c.PostFormArray("line_work_type_id"),
		c.PostFormArray("line_name"),
		c.PostFormArray("line_quantity"),
		c.PostFormArray("line_net"),
	)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	data := Data{
		Plate:   c.PostForm("plate"),
		Date:    c.PostForm("date"),
		Mileage: c.PostForm("mileage"),
		Status:  c.PostForm("status"),
		Comment: c.PostForm("comment"),
		Works:   c.PostFormArray("works"),
		Lines:   lines,
	}

	result, err := Attach(h.db, data)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	url := fmt.Sprintf("/vehicles/%d", result.VehicleID)
	c.Redirect(http.StatusSeeOther, url)
}
