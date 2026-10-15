package edit

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

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
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	repair, err := store.GetRepair(h.db, id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	vehicle, err := store.GetVehicle(h.db, repair.VehicleID)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	types, err := store.AllWorkTypes(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	selectedWorkIDs, err := store.GetRepairWorkTypeIDs(h.db, id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	// Create a map for quick lookup in template
	selectedMap := make(map[int64]bool)
	for _, wid := range selectedWorkIDs {
		selectedMap[wid] = true
	}

	invoice, err := store.GetInvoice(h.db, id)
	if err == sql.ErrNoRows {
		invoice = nil
	} else if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	settings, err := store.GetInvoiceSettings(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	data := map[string]interface{}{
		"Repair":      repair,
		"Plate":       vehicle.PlateNumber,
		"Date":        repair.RepairDate.Format("2006-01-02"),
		"Status":      repair.Status,
		"VATRate":     settings.VATRate,
		"WorkTypes":   types,
		"WorkGroups":  repairform.Groups(types),
		"SelectedIDs": selectedMap,
		"Invoice":     invoice,
	}

	c.HTML(http.StatusOK, "repair_form", data)
}

func (h *handlers) DoUpdate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

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

	if err := Update(h.db, id, data); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	repair, err := store.GetRepair(h.db, id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	url := fmt.Sprintf("/vehicles/%d", repair.VehicleID)
	c.Redirect(http.StatusSeeOther, url)
}

func (h *handlers) DoDelete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	repair, err := store.GetRepair(h.db, id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	vid := repair.VehicleID

	if err := Remove(h.db, id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	url := fmt.Sprintf("/vehicles/%d", vid)
	c.Redirect(http.StatusSeeOther, url)
}

func (h *handlers) DoPay(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	if err := store.MarkRepairPaid(h.db, id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/")
}
