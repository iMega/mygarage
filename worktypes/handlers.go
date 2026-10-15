package worktypes

import (
	"database/sql"
	"html/template"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/store"
)

type handlers struct {
	db   *sql.DB
	tmpl *template.Template
}

func Handlers(db *sql.DB, tmpl *template.Template) *handlers {
	return &handlers{db: db, tmpl: tmpl}
}

func (h *handlers) ListAll(c *gin.Context) {
	show := filter(c.Query("show"))
	list, err := List(h.db, show)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	data := map[string]interface{}{
		"SettingsPage": "works",
		"WorkTypes":    list,
		"Show":         show,
	}

	c.HTML(http.StatusOK, "worktypes", data)
}

func (h *handlers) DoCreate(c *gin.Context) {
	order, err := NextSortOrder(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	price, err := store.ParseMoney(c.PostForm("default_price"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid default price")
		return
	}

	data := Data{
		Category:          c.PostForm("category"),
		Name:              c.PostForm("name"),
		InvoiceNameEN:     c.PostForm("invoice_name_en"),
		DefaultPriceCents: price,
		SortOrder:         order,
	}

	if _, err := Create(h.db, data); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/work-types")
}

func (h *handlers) DoUpdate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	order, err := SortOrder(h.db, id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	price, err := store.ParseMoney(c.PostForm("default_price"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid default price")
		return
	}

	data := Data{
		Category:          c.PostForm("category"),
		Name:              c.PostForm("name"),
		InvoiceNameEN:     c.PostForm("invoice_name_en"),
		DefaultPriceCents: price,
		SortOrder:         order,
	}

	if err := Update(h.db, id, data); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	redirect(c)
}

func (h *handlers) DoDisable(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	if err := Disable(h.db, id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	redirect(c)
}

func (h *handlers) DoEnable(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	if err := Enable(h.db, id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	redirect(c)
}

func (h *handlers) DoMoveUp(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	if err := MoveUp(h.db, id); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	redirect(c)
}

func filter(show string) string {
	switch show {
	case "hidden", "all", "uncategorized":
		return show
	default:
		return "active"
	}
}

func redirect(c *gin.Context) {
	show := filter(c.Query("show"))
	if show == "active" {
		c.Redirect(http.StatusSeeOther, "/work-types")
		return
	}
	c.Redirect(http.StatusSeeOther, "/work-types?show="+show)
}
