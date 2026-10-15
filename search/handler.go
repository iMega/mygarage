package search

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

func Handler(db *sql.DB, _ *template.Template) gin.HandlerFunc {
	return func(c *gin.Context) {
		plate := c.Query("plate")

		result, err := Find(db, plate)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		// If vehicle found, redirect to its history page
		if result.Found {
			url := fmt.Sprintf("/vehicles/%d", result.Vehicle.ID)

			// If this is an HTMX request, use HX-Redirect header
			// Otherwise use standard redirect
			if c.GetHeader("HX-Request") == "true" {
				c.Header("HX-Redirect", url)
				c.Status(http.StatusOK)
				return
			}

			c.Redirect(http.StatusSeeOther, url)
			return
		}

		next := "/repairs/new?plate=" + url.QueryEscape(plate)
		if c.GetHeader("HX-Request") == "true" {
			c.Header("HX-Redirect", next)
			c.Status(http.StatusOK)
			return
		}

		c.Redirect(http.StatusSeeOther, next)
	}
}
