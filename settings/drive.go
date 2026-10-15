package settings

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/appdir"
	"github.com/imega/mygarage/backup"
	"github.com/imega/mygarage/googledrive"
)

func (h *handlers) SaveBackup(c *gin.Context) {
	local := c.PostForm("local") == "1"
	drive := c.PostForm("drive") == "1"
	if drive && !driveConnected() {
		c.String(http.StatusBadRequest, "Google Drive is not connected")
		return
	}
	if err := backup.SaveSettings(h.db, local, drive); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, "/settings/data?saved=1")
}

func (h *handlers) SaveDriveClient(c *gin.Context) {
	path, err := appdir.GoogleDriveClient()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	file, err := c.FormFile("client")
	if err != nil {
		c.String(http.StatusBadRequest, "client file is required")
		return
	}
	src, err := file.Open()
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	defer src.Close()

	data, err := io.ReadAll(io.LimitReader(src, 1024*1024+1))
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if _, err := googledrive.SaveClient(path, data); err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	// A refresh token belongs to the previous client and stops working.
	if err := h.disconnectDrive(); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/access?drive=saved")
}

func (h *handlers) ConnectDrive(c *gin.Context) {
	authURL, err := h.drive.AuthURL(h.driveRedirectURI(c))
	if err != nil {
		redirectDriveError(c, err)
		return
	}
	c.Redirect(http.StatusFound, authURL)
}

func (h *handlers) DriveCallback(c *gin.Context) {
	if reason := c.Query("error"); reason != "" {
		redirectDriveError(c, &driveDenied{reason: reason})
		return
	}
	err := h.drive.Complete(
		c.Request.Context(), c.Query("state"), c.Query("code"),
	)
	if err != nil {
		redirectDriveError(c, err)
		return
	}
	c.Redirect(http.StatusSeeOther, "/settings/access?drive=connected")
}

func (h *handlers) DisconnectDrive(c *gin.Context) {
	if err := h.disconnectDrive(); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, "/settings/access")
}

func (h *handlers) DeleteDriveClient(c *gin.Context) {
	path, err := appdir.GoogleDriveClient()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.disconnectDrive(); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Redirect(http.StatusSeeOther, "/settings/access")
}

// disconnectDrive removes the token and turns off Drive backups, so backups
// do not fail silently on every start.
func (h *handlers) disconnectDrive() error {
	if err := h.drive.Disconnect(); err != nil {
		return err
	}
	settings, err := backup.GetSettings(h.db)
	if err != nil {
		return err
	}
	return backup.SaveSettings(h.db, settings.Local, false)
}

type driveDenied struct {
	reason string
}

func (e *driveDenied) Error() string {
	return "Google denied access: " + e.reason
}

func redirectDriveError(c *gin.Context, err error) {
	c.Redirect(
		http.StatusSeeOther,
		"/settings/access?drive_error="+url.QueryEscape(err.Error()),
	)
}

// driveRedirectURI points to the public URL when the server runs behind a
// proxy. Otherwise it keeps the port the browser used but always points to
// the loopback address, which Desktop OAuth clients accept without
// registration.
func (h *handlers) driveRedirectURI(c *gin.Context) string {
	if h.publicURL != "" {
		return h.publicURL + "/settings/drive/callback"
	}
	_, port, err := net.SplitHostPort(c.Request.Host)
	if err != nil {
		port = "80"
	}
	return "http://127.0.0.1:" + port + "/settings/drive/callback"
}

func driveConnected() bool {
	path, err := appdir.GoogleDriveToken()
	if err != nil {
		return false
	}
	return googledrive.Connected(path)
}
