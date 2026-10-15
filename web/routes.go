package web

import (
	"database/sql"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/appdir"
	"github.com/imega/mygarage/attach"
	"github.com/imega/mygarage/backup"
	garagecalendar "github.com/imega/mygarage/calendar"
	"github.com/imega/mygarage/docs"
	"github.com/imega/mygarage/edit"
	"github.com/imega/mygarage/export"
	"github.com/imega/mygarage/googledrive"
	"github.com/imega/mygarage/history"
	"github.com/imega/mygarage/search"
	"github.com/imega/mygarage/settings"
	"github.com/imega/mygarage/shutdown"
	"github.com/imega/mygarage/worktypes"
)

// Options selects the run mode of the web layer.
type Options struct {
	// Daemon disables desktop-only routes: shutdown and init.sql upload.
	Daemon bool
	// PublicURL is the external base URL for OAuth callbacks; empty means
	// the loopback address.
	PublicURL string
	// Ready gates /readyz; nil means always ready.
	Ready *atomic.Bool
	// Shutdown handles POST /shutdown; nil leaves the route unregistered.
	Shutdown *shutdown.Manager
}

func Setup(
	db *sql.DB, dbPath string, drive *googledrive.Connector, opts Options,
) (*gin.Engine, error) {
	tmpl, err := LoadTemplates(opts.Daemon)
	if err != nil {
		return nil, err
	}

	r := gin.Default()
	r.SetHTMLTemplate(tmpl)

	assetsDir, err := appdir.Assets()
	if err != nil {
		return nil, err
	}
	r.Static("/assets", assetsDir)

	attachH := attach.Handlers(db, tmpl)
	editH := edit.Handlers(db, tmpl)
	worktypesH := worktypes.Handlers(db, tmpl)
	docsH := docs.Handlers(db, tmpl)
	settingsH := settings.Handlers(
		db, dbPath, tmpl, drive, opts.PublicURL,
	)
	credentialsPath, err := appdir.GoogleCalendarCredentials()
	if err != nil {
		return nil, err
	}
	agenda := garagecalendar.New(db, credentialsPath)

	r.GET("/healthz", Healthz)
	r.GET("/readyz", Readyz(db, opts.Ready))
	r.GET("/version", Version)

	r.GET("/", Home(db, tmpl, agenda))
	r.GET("/history", history.ListHandler(db, tmpl))
	r.GET("/vehicles/search", search.Handler(db, tmpl))
	r.GET("/vehicles/:id", history.Handler(db, tmpl))

	r.GET("/repairs/new", attachH.ShowForm)
	r.POST("/repairs", attachH.Create)

	r.GET("/repairs/:id/edit", editH.ShowForm)
	r.POST("/repairs/:id", editH.DoUpdate)
	r.POST("/repairs/:id/pay", editH.DoPay)
	r.POST("/repairs/:id/delete", editH.DoDelete)
	r.GET("/repairs/:id/certificate", docsH.Certificate)
	r.GET("/repairs/:id/invoice", docsH.Invoice)
	r.GET("/repairs/:id/documents", docsH.Documents)

	r.GET("/work-types", worktypesH.ListAll)
	r.POST("/work-types", worktypesH.DoCreate)
	r.POST("/work-types/:id", worktypesH.DoUpdate)
	r.POST("/work-types/:id/disable", worktypesH.DoDisable)
	r.POST("/work-types/:id/enable", worktypesH.DoEnable)
	r.POST("/work-types/:id/move-up", worktypesH.DoMoveUp)

	r.GET("/settings/company", settingsH.Show)
	r.POST("/settings/company", settingsH.Save)
	r.GET("/settings/data", settingsH.Data)
	r.GET("/settings/access", settingsH.Access)
	r.POST("/settings/calendar", settingsH.SaveCalendar)
	r.POST(
		"/settings/calendar/credentials/delete",
		settingsH.DeleteCalendarCredentials,
	)
	r.POST("/settings/backup", settingsH.SaveBackup)
	r.POST("/settings/drive", settingsH.SaveDriveClient)
	r.GET("/settings/drive/connect", settingsH.ConnectDrive)
	r.GET("/settings/drive/callback", settingsH.DriveCallback)
	r.POST("/settings/drive/disconnect", settingsH.DisconnectDrive)
	r.POST("/settings/drive/client/delete", settingsH.DeleteDriveClient)
	if !opts.Daemon {
		r.POST("/settings/init-sql", settingsH.UploadInitSQL)
	}
	r.POST("/settings/assets/:kind", settingsH.UploadAsset)
	r.POST("/settings/assets/:kind/delete", settingsH.DeleteAsset)

	r.GET("/export/repairs.csv", export.Handler(db))

	r.POST("/backup/create", func(c *gin.Context) {
		if err := backup.Create(c.Request.Context(), db, drive); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Redirect(http.StatusSeeOther, "/settings/data")
	})

	if !opts.Daemon && opts.Shutdown != nil {
		r.POST("/shutdown", opts.Shutdown.Handler())
	}

	return r, nil
}
