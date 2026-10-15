package web

import (
	"context"
	"database/sql"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/version"
)

// Healthz reports that the process is alive.
func Healthz(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// Readyz reports whether the instance may receive traffic: it is not
// draining and the database answers.
func Readyz(db *sql.DB, ready *atomic.Bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ready != nil && !ready.Load() {
			c.String(http.StatusServiceUnavailable, "draining")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			c.String(http.StatusServiceUnavailable, "db: "+err.Error())
			return
		}

		c.String(http.StatusOK, "ready")
	}
}

// Version reports build metadata of the running binary.
func Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"sha":       version.SHA,
		"buildTime": version.BuildTime,
	})
}
