package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/imega/mygarage/appdir"
	"github.com/imega/mygarage/backup"
	"github.com/imega/mygarage/browser"
	"github.com/imega/mygarage/config"
	"github.com/imega/mygarage/db"
	"github.com/imega/mygarage/googledrive"
	"github.com/imega/mygarage/lockfile"
	"github.com/imega/mygarage/shutdown"
	"github.com/imega/mygarage/version"
	"github.com/imega/mygarage/web"
)

// Shutdown timeouts: a desktop user is waiting for the window to close,
// a daemon drains in-flight requests and must stay under systemd
// TimeoutStopSec.
const (
	desktopShutdownTimeout = 5 * time.Second
	daemonShutdownTimeout  = 25 * time.Second
)

type options struct {
	daemon    bool
	port      int
	dataDir   string
	publicURL string
}

// parseOptions resolves run options: a command-line flag wins over an
// environment variable, which wins over config.json.
func parseOptions() options {
	var opts options
	flag.BoolVar(&opts.daemon, "daemon", false,
		"run as a server: no lockfile, browser or shutdown endpoint")
	flag.IntVar(&opts.port, "port", 0, "listen port on 127.0.0.1")
	flag.StringVar(&opts.dataDir, "data-dir", "", "data directory")
	flag.StringVar(&opts.publicURL, "public-url", "",
		"external base URL for OAuth callbacks")
	flag.Parse()

	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	if !set["daemon"] {
		opts.daemon, _ = strconv.ParseBool(os.Getenv("MYGARAGE_DAEMON"))
	}
	if !set["port"] {
		opts.port, _ = strconv.Atoi(os.Getenv("MYGARAGE_PORT"))
	}
	if !set["data-dir"] {
		opts.dataDir = os.Getenv("MYGARAGE_DATA_DIR")
	}
	if !set["public-url"] {
		opts.publicURL = os.Getenv("MYGARAGE_PUBLIC_URL")
	}

	return opts
}

func main() {
	opts := parseOptions()
	if opts.dataDir != "" {
		appdir.SetRoot(opts.dataDir)
	}

	configPath, err := appdir.Config()
	if err != nil {
		log.Fatalf("appdir config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config load: %v", err)
	}
	if opts.port != 0 {
		cfg.Port = opts.port
	}
	if opts.publicURL != "" {
		cfg.PublicURL = opts.publicURL
	}
	if opts.daemon {
		cfg.OpenBrowser = false
	}

	log.Printf("myGarage version %s (built %s), daemon=%t",
		version.SHA, version.BuildTime, opts.daemon)

	// Blue/green slots share one data directory, so the single-instance
	// lock applies to the desktop mode only.
	if !opts.daemon {
		lock, err := lockfile.Acquire()
		if err != nil {
			log.Printf("Another instance is running")
			if os.Getenv("MYGARAGE_NO_BROWSER") != "1" {
				url := browser.URL(cfg.Port)
				if err := browser.Open(url); err != nil {
					log.Printf("Failed to open browser: %v", err)
				}
			}
			os.Exit(0)
		}
		defer lock.Release()
	}

	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath, err = appdir.DB()
		if err != nil {
			log.Fatalf("appdir db: %v", err)
		}
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	drive, err := driveConnector()
	if err != nil {
		log.Fatalf("appdir google drive: %v", err)
	}

	startupBackup := make(chan struct{})
	go func() {
		defer close(startupBackup)
		ctx, cancel := context.WithTimeout(
			context.Background(), backup.FinalTimeout,
		)
		defer cancel()
		if err := backup.Create(ctx, database, drive); err != nil {
			log.Printf("backup create: %v", err)
		}
	}()

	sm := shutdown.New(database, drive)
	var ready atomic.Bool

	r, err := web.Setup(database, dbPath, drive, web.Options{
		Daemon:    opts.daemon,
		PublicURL: cfg.PublicURL,
		Ready:     &ready,
		Shutdown:  sm,
	})
	if err != nil {
		log.Fatalf("web setup: %v", err)
	}

	url := browser.URL(cfg.Port)
	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)

	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		select {
		case <-sigCh:
			log.Println("Signal received, shutting down...")
			ready.Store(false)

			if opts.daemon {
				// The other slot keeps serving the same database: no
				// exclusive checkpoint and no final backup here.
				break
			}

			<-startupBackup

			if _, err := database.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
				log.Printf("WAL checkpoint: %v", err)
			}

			backupCtx, cancel := context.WithTimeout(
				context.Background(), backup.FinalTimeout,
			)
			if err := backup.Create(backupCtx, database, drive); err != nil {
				log.Printf("Final backup: %v", err)
			}
			cancel()

			if err := database.Close(); err != nil {
				log.Printf("DB close: %v", err)
			}
		case <-sm.Done():
			log.Println("Shutdown requested from web...")
		}

		timeout := desktopShutdownTimeout
		if opts.daemon {
			timeout = daemonShutdownTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown: %v", err)
		}

		if opts.daemon {
			<-startupBackup
			if err := database.Close(); err != nil {
				log.Printf("DB close: %v", err)
			}
		}
	}()

	if cfg.OpenBrowser && os.Getenv("MYGARAGE_NO_BROWSER") != "1" {
		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := browser.Open(url); err != nil {
				log.Printf("browser open: %v", err)
			}
		}()
	}

	root, _ := appdir.Root()
	log.Printf("Data directory: %s", root)
	log.Printf("Starting server at %s", url)

	ready.Store(true)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server run: %v", err)
	}

	// ListenAndServe returns as soon as Shutdown starts; wait until
	// in-flight requests are drained.
	<-stopped

	log.Println("Server stopped")
}

func driveConnector() (*googledrive.Connector, error) {
	clientPath, err := appdir.GoogleDriveClient()
	if err != nil {
		return nil, err
	}
	tokenPath, err := appdir.GoogleDriveToken()
	if err != nil {
		return nil, err
	}
	return googledrive.NewConnector(clientPath, tokenPath), nil
}
