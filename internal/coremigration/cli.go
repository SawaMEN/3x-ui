package coremigration

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
)

func CLI(args []string) error { return CLIWithInitializer(args, nil) }

// CLIWithInitializer lets main load the same node-token keyring as the daemon.
func CLIWithInitializer(args []string, initialize func() error) error {
	f := flag.NewFlagSet("core-migrate", flag.ContinueOnError)
	var o Options
	var binaryOnly, verifyRunning bool
	f.BoolVar(&verifyRunning, "verify-running", false, "verify the restarted panel owns the pinned sing-box process")
	f.BoolVar(&binaryOnly, "check-binary", false, "verify the pinned owner binary without opening the database")
	f.StringVar(&o.BackupDir, "backup-dir", "", "private persistent snapshot directory (must be new, or prepared by --backup-only)")
	f.BoolVar(&o.BackupOnly, "backup-only", false, "snapshot the stopped installation before replacing panel files")
	f.BoolVar(&o.Restore, "restore", false, "restore this installation's migration snapshot")
	f.SetOutput(io.Discard)
	if err := f.Parse(args); err != nil {
		return err
	}
	modes := 0
	for _, enabled := range []bool{o.Restore, o.BackupOnly, binaryOnly, verifyRunning} {
		if enabled {
			modes++
		}
	}
	if len(f.Args()) != 0 || modes > 1 || (binaryOnly || verifyRunning) && o.BackupDir != "" {
		return fmt.Errorf("invalid core-migrate arguments")
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 5*time.Minute)
	defer cancel()
	if verifyRunning {
		process := hiddify.NewProcess(hiddify.GetConfigPath(), false)
		if _, err := process.Version(ctx); err != nil {
			return err
		}
		deadline := time.NewTimer(30 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			if process.IsRunning() {
				return nil
			}
			select {
			case <-deadline.C:
				return fmt.Errorf("restarted panel has no owner sing-box process")
			case <-ctx.Done():
				return ctx.Err()
			case <-tick.C:
			}
		}
	}
	if binaryOnly {
		_, err := hiddify.NewProcess("", true).Version(ctx)
		return err
	}
	if !o.BackupOnly && !o.Restore && initialize != nil {
		if err := initialize(); err != nil {
			return err
		}
	}
	if err := Run(ctx, o); err != nil {
		return err
	}
	fmt.Printf("core migration: sing-box; snapshot: %s\n", o.BackupDir)
	return nil
}
