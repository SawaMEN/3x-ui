package controller

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func (a *AdBlockController) validateServerCore(ctx context.Context) error {
	cfg, err := service.SelectedNativeCore().GetConfig()
	if err != nil {
		return err
	}
	raw, err := cfg.Marshal()
	if err != nil {
		return err
	}
	binary := hiddify.GetBinaryPath()
	args := []string{"check", "-c"}
	file, err := os.CreateTemp("", "x-ui-adblock-check-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, binary, append(args, file.Name())...)
	command.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+config.GetBinFolderPath())
	output, err := command.CombinedOutput()
	if err != nil {
		if len(output) > 4096 {
			output = output[:4096]
		}
		return fmt.Errorf("core validation: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
