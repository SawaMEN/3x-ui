package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/recovery"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func recoveryCLI(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: x-ui recovery export|check|restore --file <bundle> [--out <new-directory>] --password-file <mode-0600-file>")
	}
	flags := flag.NewFlagSet("recovery", flag.ContinueOnError)
	file := flags.String("file", "", "encrypted recovery bundle")
	out := flags.String("out", "", "new output directory for offline restore")
	secret := flags.String("password-file", "", "mode-0600 file containing the recovery password")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("provide --file")
	}
	password := os.Getenv("XUI_RECOVERY_PASSWORD")
	if *secret != "" {
		info, err := os.Stat(*secret)
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o077 != 0 || !info.Mode().IsRegular() || info.Size() > 4096 {
			return errors.New("password file must be mode 0600 or stricter and at most 4096 bytes")
		}
		raw, err := os.ReadFile(*secret)
		if err != nil {
			return err
		}
		password = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	}
	if len(password) < 16 {
		return errors.New("set XUI_RECOVERY_PASSWORD or provide a password file with at least 16 bytes")
	}
	switch args[0] {
	case "export":
		if err := initNodeTokenCrypto(); err != nil {
			return err
		}
		if err := database.InitDB(config.GetDBPath()); err != nil {
			return err
		}
		defer func() { _ = database.CloseDB() }()
		data, err := service.CreateRecoveryBundle(context.Background(), password)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	case "check", "restore":
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, recovery.MaxSize+1))
		if err != nil {
			return err
		}
		bundle, err := recovery.Decode(data, password)
		if err != nil {
			return err
		}
		if args[0] == "check" {
			fmt.Printf("Recovery verified: %d files; database=%s; panel=%s; core=%s\n", len(bundle.Files), bundle.DatabaseType, bundle.PanelVersion, bundle.CoreType)
			return nil
		}
		return recovery.Extract(bundle, *out)
	default:
		return errors.New("unknown recovery operation")
	}
}
