package settings

import (
	"database/sql"
	"html/template"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/appdir"
	"github.com/imega/mygarage/backup"
	garagecalendar "github.com/imega/mygarage/calendar"
	"github.com/imega/mygarage/googledrive"
	"github.com/imega/mygarage/store"
)

type handlers struct {
	db     *sql.DB
	dbPath string
	tmpl   *template.Template
	drive  *googledrive.Connector
	// publicURL is the external base URL for OAuth callbacks; empty means
	// the loopback address.
	publicURL string
}

func Handlers(
	db *sql.DB, dbPath string, tmpl *template.Template,
	drive *googledrive.Connector, publicURL string,
) *handlers {
	return &handlers{
		db: db, dbPath: dbPath, tmpl: tmpl, drive: drive,
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

func (h *handlers) Show(c *gin.Context) {
	company, err := store.GetCompany(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	invoice, err := store.GetInvoiceSettings(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	data := map[string]interface{}{
		"SettingsPage":       "documents",
		"Company":            company,
		"Invoice":            invoice,
		"NextInvoiceNumber":  number(invoice),
		"PrintInvoiceNumber": printNumber(invoice),
		"HasSignature":       hasAsset("signature"),
		"HasStamp":           hasAsset("stamp"),
	}
	c.HTML(http.StatusOK, "settings", data)
}

func (h *handlers) Data(c *gin.Context) {
	dir, err := appdir.Backups()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	backupSettings, err := backup.GetSettings(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	data := map[string]interface{}{
		"SettingsPage":   "data",
		"DBPath":         h.dbPath,
		"BackupDir":      dir,
		"AssetsDir":      assetsDir(),
		"InitSQLPath":    initSQLPath(),
		"HasInitSQL":     hasInitSQL(),
		"LastBackup":     lastBackup(dir),
		"Backup":         backupSettings,
		"DriveConnected": driveConnected(),
		"Saved":          c.Query("saved") == "1",
	}
	c.HTML(http.StatusOK, "settings_data", data)
}

func (h *handlers) Access(c *gin.Context) {
	settings, err := garagecalendar.GetSettings(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	path, err := appdir.GoogleCalendarCredentials()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	email, err := garagecalendar.ReadCredentials(path)
	if err != nil && !os.IsNotExist(err) {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	driveClientPath, err := appdir.GoogleDriveClient()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	driveClientID, err := googledrive.ReadClient(driveClientPath)
	if err != nil && !os.IsNotExist(err) {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	data := map[string]interface{}{
		"SettingsPage":    "access",
		"Calendar":        settings,
		"CredentialEmail": email,
		"HasCredentials":  email != "",
		"Saved":           c.Query("saved") == "1",
		"DriveClientID":   driveClientID,
		"HasDriveClient":  driveClientID != "",
		"DriveConnected":  driveConnected(),
		"DriveSaved":      c.Query("drive") == "saved",
		"DriveLinked":     c.Query("drive") == "connected",
		"DriveError":      c.Query("drive_error"),
	}
	c.HTML(http.StatusOK, "settings_access", data)
}

func (h *handlers) SaveCalendar(c *gin.Context) {
	path, err := appdir.GoogleCalendarCredentials()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	file, err := c.FormFile("credentials")
	if err == nil && file.Size > 0 {
		src, openErr := file.Open()
		if openErr != nil {
			c.String(http.StatusBadRequest, openErr.Error())
			return
		}
		defer src.Close()

		data, readErr := io.ReadAll(io.LimitReader(src, 1024*1024+1))
		if readErr != nil {
			c.String(http.StatusBadRequest, readErr.Error())
			return
		}
		if _, saveErr := garagecalendar.SaveCredentials(path, data); saveErr != nil {
			c.String(http.StatusBadRequest, saveErr.Error())
			return
		}
	}

	enabled := c.PostForm("enabled") == "1"
	calendarID := c.PostForm("calendar_id")
	if enabled {
		if strings.TrimSpace(calendarID) == "" {
			c.String(http.StatusBadRequest, "calendar ID is required")
			return
		}
		if _, err := garagecalendar.ReadCredentials(path); err != nil {
			c.String(http.StatusBadRequest, "credentials file is required")
			return
		}
	}
	if err := garagecalendar.SaveSettings(
		h.db, enabled, calendarID,
	); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/access?saved=1")
}

func (h *handlers) DeleteCalendarCredentials(c *gin.Context) {
	path, err := appdir.GoogleCalendarCredentials()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	settings, err := garagecalendar.GetSettings(h.db)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if err := garagecalendar.SaveSettings(
		h.db, false, settings.CalendarID,
	); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/access")
}

func (h *handlers) Save(c *gin.Context) {
	next, err := strconv.ParseInt(c.PostForm("next_number"), 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid next number")
		return
	}

	padding, err := strconv.Atoi(c.PostForm("padding"))
	if err != nil {
		c.String(http.StatusBadRequest, "invalid padding")
		return
	}

	rate := store.DefaultVATRate
	if raw := c.PostForm("vat_rate"); raw != "" {
		rate, err = strconv.Atoi(raw)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid VAT rate")
			return
		}
	}

	company := store.Company{
		Name:                     c.PostForm("name"),
		Address:                  c.PostForm("address"),
		FPANumber:                c.PostForm("fpa_number"),
		AFTNumber:                c.PostForm("aft_number"),
		Phone:                    c.PostForm("phone"),
		Email:                    c.PostForm("email"),
		TechnicianName:           c.PostForm("technician_name"),
		TechnicianSpeciality:     c.PostForm("technician_speciality"),
		TechnicianRegistryNumber: c.PostForm("technician_registry_number"),
	}

	if err := store.SaveCompany(h.db, company); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	err = store.SaveInvoiceSettings(
		h.db, c.PostForm("prefix"), next, padding, rate,
	)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/company")
}

func (h *handlers) UploadAsset(c *gin.Context) {
	kind := c.Param("kind")
	path, ok := assetPath(kind)
	if !ok {
		c.String(http.StatusBadRequest, "invalid asset")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "file is required")
		return
	}

	src, err := file.Open()
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	defer src.Close()

	img, _, err := image.Decode(src)
	if err != nil {
		c.String(http.StatusBadRequest, "unsupported image format")
		return
	}

	dst, err := os.Create(path)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	defer dst.Close()

	if err := png.Encode(dst, img); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/company")
}

func (h *handlers) DeleteAsset(c *gin.Context) {
	path, ok := assetPath(c.Param("kind"))
	if !ok {
		c.String(http.StatusBadRequest, "invalid asset")
		return
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/company")
}

func (h *handlers) UploadInitSQL(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "file is required")
		return
	}
	if strings.ToLower(filepath.Ext(file.Filename)) != ".sql" {
		c.String(http.StatusBadRequest, "file must have .sql extension")
		return
	}

	src, err := file.Open()
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	defer src.Close()

	path, err := appdir.InitSQL()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	dst, err := os.Create(path)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/data")
}

func number(s *store.InvoiceSettings) string {
	if s == nil {
		return ""
	}
	return store.InvoiceNumber(s.Prefix, s.NextNumber, s.Padding)
}

func printNumber(s *store.InvoiceSettings) string {
	if s == nil {
		return ""
	}
	return store.Invoice{Number: number(s)}.PrintNumber()
}

func lastBackup(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}

	sort.Strings(names)
	return names[len(names)-1]
}

func assetsDir() string {
	dir, err := appdir.Assets()
	if err != nil {
		return ""
	}
	return dir
}

func initSQLPath() string {
	path, err := appdir.InitSQL()
	if err != nil {
		return ""
	}
	return path
}

func hasInitSQL() bool {
	path, err := appdir.InitSQL()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func hasAsset(kind string) bool {
	path, ok := assetPath(kind)
	if !ok {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func assetPath(kind string) (string, bool) {
	name := ""
	switch kind {
	case "signature":
		name = "signature.png"
	case "stamp":
		name = "stamp.png"
	default:
		return "", false
	}

	dir, err := appdir.Assets()
	if err != nil {
		return "", false
	}
	return filepath.Join(dir, name), true
}
