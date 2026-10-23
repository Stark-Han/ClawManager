// system-backup-controller owns installation-local coordination and safe
// unstarted-task recovery. It does not start the application or run migrations.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"clawreef/internal/systembackup"
	"github.com/go-sql-driver/mysql"
)

func run(ctx context.Context) error {
	path := os.Getenv("SYSTEM_BACKUP_CONTROL_DSN_FILE")
	identity := os.Getenv("SYSTEM_BACKUP_INSTALLATION_ID")
	owner := os.Getenv("SYSTEM_BACKUP_CONTROLLER_ID")
	if path == "" || identity == "" || owner == "" {
		return errors.New("controller bootstrap configuration missing")
	}
	info, e := os.Stat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return errors.New("controller credential unavailable")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return errors.New("controller credential unavailable")
	}
	defer clear(raw)
	config, e := mysql.ParseDSN(strings.TrimSpace(string(raw)))
	if e != nil || config.DBName == "" {
		return errors.New("controller database configuration invalid")
	}
	config.ParseTime = true
	config.Timeout = 5 * time.Second
	config.ReadTimeout = 10 * time.Second
	config.WriteTimeout = 10 * time.Second
	db, e := sql.Open("mysql", config.FormatDSN())
	if e != nil {
		return errors.New("controller database unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	if e = db.PingContext(ctx); e != nil {
		return errors.New("controller database unavailable")
	}
	store := systembackup.Store{DB: db, InstallationID: identity}
	if e = store.Bootstrap(ctx); e != nil {
		return errors.New("controller identity or schema unavailable")
	}
	controller := systembackup.Controller{Store: store, Owner: owner, PollInterval: 5 * time.Second}
	return controller.Run(ctx)
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if e := run(ctx); e != nil && !errors.Is(e, context.Canceled) {
		log.Print("system backup controller stopped: ", systembackup.PublicError(e))
		os.Exit(1)
	}
}
