// system-backup-controller owns installation-local coordination and safe
// unstarted-task recovery. It does not start the application or run migrations.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"clawreef/internal/systembackup"
)

func run(ctx context.Context) error {
	path := os.Getenv("SYSTEM_BACKUP_CONTROL_DSN_FILE")
	identity := os.Getenv("SYSTEM_BACKUP_INSTALLATION_ID")
	owner := os.Getenv("SYSTEM_BACKUP_CONTROLLER_ID")
	if path == "" || identity == "" || owner == "" {
		return errors.New("controller bootstrap configuration missing")
	}
	db, e := systembackup.OpenControlDatabase(ctx, path)
	if e != nil {
		return e
	}
	defer db.Close()
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
