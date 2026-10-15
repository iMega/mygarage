package shutdown

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imega/mygarage/backup"
)

type Manager struct {
	db       *sql.DB
	drive    backup.Uploader
	shutdown chan struct{}
}

func New(db *sql.DB, drive backup.Uploader) *Manager {
	return &Manager{
		db:       db,
		drive:    drive,
		shutdown: make(chan struct{}),
	}
}

func (m *Manager) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Работа завершена - myGarage</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
    <style>
        body {
            display: flex;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            background: #f8f9fa;
        }
    </style>
</head>
<body>
    <div class="text-center">
        <h1 class="mb-4">Работа завершена</h1>
        <p class="text-muted mb-4">
            База данных сохранена.<br>
            Можно закрыть это окно.
        </p>
        <div class="spinner-border text-success" role="status">
            <span class="visually-hidden">Завершение...</span>
        </div>
    </div>
</body>
</html>`)

		go func() {
			time.Sleep(300 * time.Millisecond)

			log.Println("Graceful shutdown initiated...")

			if _, err := m.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
				log.Printf("WAL checkpoint: %v", err)
			}

			ctx, cancel := context.WithTimeout(
				context.Background(), backup.FinalTimeout,
			)
			if err := backup.Create(ctx, m.db, m.drive); err != nil {
				log.Printf("Final backup: %v", err)
			}
			cancel()

			if err := m.db.Close(); err != nil {
				log.Printf("DB close: %v", err)
			}

			close(m.shutdown)
		}()
	}
}

func (m *Manager) Wait(ctx context.Context) {
	select {
	case <-m.shutdown:
		log.Println("Shutdown completed")
	case <-ctx.Done():
		log.Println("Shutdown timeout")
	}
}

func (m *Manager) Done() <-chan struct{} {
	return m.shutdown
}
