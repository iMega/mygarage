package docs

import (
	"database/sql"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/appdir"
	"github.com/imega/mygarage/store"
)

type handlers struct {
	db   *sql.DB
	tmpl *template.Template
}

type Data struct {
	Repair        *store.Repair
	Vehicle       *store.Vehicle
	Company       *store.Company
	Invoice       *store.Invoice
	Settings      *store.InvoiceSettings
	AutoPrint     bool
	Signature     string
	Stamp         string
	InvoiceRows   []InvoiceRow
	InvoiceCopies []int
}

type InvoiceRow struct {
	Quantity string
	Name     string
	Rate     string
	Euro     string
	Cent     string
}

func (d Data) TechnicianName() string {
	if d.Company != nil &&
		strings.TrimSpace(d.Company.TechnicianName) != "" {
		return d.Company.TechnicianName
	}
	if d.Company != nil && strings.TrimSpace(d.Company.Name) != "" {
		return d.Company.Name
	}
	return ""
}

func (d Data) TechnicianSpeciality() string {
	if d.Company != nil &&
		strings.TrimSpace(d.Company.TechnicianSpeciality) != "" {
		return d.Company.TechnicianSpeciality
	}
	return ""
}

func (d Data) TechnicianRegistryNumber() string {
	if d.Company != nil &&
		strings.TrimSpace(d.Company.TechnicianRegistryNumber) != "" {
		return d.Company.TechnicianRegistryNumber
	}
	return ""
}

func (d Data) VehiclePlate() string {
	if d.Vehicle == nil {
		return ""
	}
	return strings.Join(strings.Fields(d.Vehicle.PlateNumber), "")
}

func (d Data) InvoiceNumber() string {
	if d.Invoice == nil {
		return ""
	}
	return d.Invoice.PrintNumber()
}

func (d Data) WorkDescription() string {
	if d.Invoice != nil && len(d.Invoice.Lines) > 0 {
		lines := make([]string, 0, len(d.Invoice.Lines))
		for _, line := range d.Invoice.Lines {
			name := strings.TrimSpace(line.Name)
			if name != "" {
				lines = append(lines, name)
			}
		}
		return strings.Join(lines, ", ")
	}

	if d.Repair != nil && len(d.Repair.WorkTypes) > 0 {
		return strings.Join(d.Repair.WorkTypes, ", ")
	}
	return ""
}

func (d Data) GasVehiclePlate() string {
	return ""
}

func (d Data) GasDescription() string {
	return ""
}

func (d Data) Notes() string {
	if d.Repair == nil {
		return ""
	}
	return d.Repair.DisplayComment()
}

func (d Data) DocumentDate() string {
	if d.Repair == nil {
		return ""
	}
	return d.Repair.RepairDate.Format("02.01.2006")
}

func (d Data) SignatureImageURL() string {
	return d.Signature
}

func (d Data) StampImageURL() string {
	return d.Stamp
}

func (d Data) VATRate() int {
	if d.Settings == nil {
		return store.DefaultVATRate
	}
	return d.Settings.VATRate
}

func Handlers(db *sql.DB, tmpl *template.Template) *handlers {
	return &handlers{db: db, tmpl: tmpl}
}

func (h *handlers) Certificate(c *gin.Context) {
	data, err := h.load(c)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	data.AutoPrint = c.Query("print") == "1"

	c.HTML(http.StatusOK, "certificate", data)
}

func (h *handlers) Invoice(c *gin.Context) {
	data, err := h.load(c)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if data.Invoice == nil {
		c.String(http.StatusNotFound, "invoice not found")
		return
	}
	data.AutoPrint = c.Query("print") == "1"
	data.InvoiceRows = rows(data.Invoice)
	data.InvoiceCopies = []int{1, 2}
	if c.Query("copies") == "1" {
		data.InvoiceCopies = []int{1}
	}

	c.HTML(http.StatusOK, "invoice", data)
}

func (h *handlers) Documents(c *gin.Context) {
	data, err := h.load(c)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if data.Invoice == nil {
		c.String(http.StatusNotFound, "invoice not found")
		return
	}
	data.AutoPrint = c.Query("print") == "1"
	data.InvoiceRows = rows(data.Invoice)
	data.InvoiceCopies = []int{1, 2}

	c.HTML(http.StatusOK, "documents", data)
}

func (h *handlers) load(c *gin.Context) (*Data, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, err
	}

	repair, err := store.GetRepair(h.db, id)
	if err != nil {
		return nil, err
	}

	vehicle, err := store.GetVehicle(h.db, repair.VehicleID)
	if err != nil {
		return nil, err
	}

	company, err := store.GetCompany(h.db)
	if err != nil {
		return nil, err
	}

	invoice, err := store.GetInvoice(h.db, id)
	if err == sql.ErrNoRows {
		invoice = nil
	} else if err != nil {
		return nil, err
	}

	settings, err := store.GetInvoiceSettings(h.db)
	if err != nil {
		return nil, err
	}

	return &Data{
		Repair:    repair,
		Vehicle:   vehicle,
		Company:   company,
		Invoice:   invoice,
		Settings:  settings,
		Signature: assetURL("signature"),
		Stamp:     assetURL("stamp"),
	}, nil
}

func assetURL(kind string) string {
	name := ""
	switch kind {
	case "signature":
		name = "signature.png"
	case "stamp":
		name = "stamp.png"
	default:
		return ""
	}

	dir, err := appdir.Assets()
	if err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return ""
	}

	return "/assets/" + name
}

func rows(invoice *store.Invoice) []InvoiceRow {
	const max = 14

	count := len(invoice.Lines)
	if count > max {
		count = max
	}

	list := make([]InvoiceRow, count)
	for i := 0; i < count && i < len(invoice.Lines); i++ {
		line := invoice.Lines[i]
		list[i] = InvoiceRow{
			Quantity: line.Quantity,
			Name:     line.Name,
			Rate:     line.Net(),
			Euro:     line.Euro(),
			Cent:     line.Cent(),
		}
	}

	return list
}
