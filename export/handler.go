package export

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
)

func Handler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := CSV(db)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header(
			"Content-Disposition",
			"attachment; filename=repairs.csv",
		)
		c.String(http.StatusOK, data)
	}
}
