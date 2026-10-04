package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// CLI uses the same networking transaction and core-specific templates as the web UI.
func CLI(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: x-ui gateway <enable|disable|status> [--lan-interface NAME --lan-ip IPv4 --lan-prefix 24 --wan-interface NAME]")
	}
	action := args[0]
	if action != "enable" && action != "disable" && action != "status" {
		return fmt.Errorf("unknown Gateway action %q", action)
	}
	var cfg service.GatewayNetworkConfig
	flags := flag.NewFlagSet("gateway "+action, flag.ContinueOnError)
	flags.StringVar(&cfg.LANInterface, "lan-interface", "", "LAN interface")
	flags.StringVar(&cfg.LANIP, "lan-ip", "", "IPv4 address assigned to LAN interface")
	flags.IntVar(&cfg.LANPrefix, "lan-prefix", 24, "LAN prefix")
	flags.StringVar(&cfg.WANInterface, "wan-interface", "", "optional WAN interface for NAT")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected Gateway arguments")
	}
	if err := ensureDatabase(); err != nil {
		return err
	}
	core, err := (&service.SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	network := &service.GatewayNetworkService{}
	if action == "status" {
		state, err := StateForCore(core)
		if err != nil {
			return err
		}
		raw, err := json.MarshalIndent(struct {
			Core     string                       `json:"core"`
			Template State                        `json:"template"`
			Network  service.GatewayNetworkStatus `json:"network"`
		}{core, state, network.Status(ctx)}, "", "  ")
		if err == nil {
			fmt.Println(string(raw))
		}
		return err
	}
	release, err := AcquireOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	before := network.Status(ctx)
	if before.Error != "" {
		return fmt.Errorf("Gateway recovery state: %s", before.Error)
	}
	states := map[string]State{}
	for _, name := range []string{service.CoreTypeXray, service.CoreTypeSingBox} {
		state, stateErr := StateForCore(name)
		if stateErr != nil {
			return stateErr
		}
		states[name] = state
	}
	changed := []string{}
	rollback := func(disabling bool, original error) error {
		recovery, done := context.WithTimeout(context.Background(), 45*time.Second)
		defer done()
		if before.Configured {
			original = errors.Join(original, network.Suspend(recovery))
		}
		for i := len(changed) - 1; i >= 0; i-- {
			if disabling {
				original = errors.Join(original, RestoreForCore(changed[i]))
			} else {
				original = errors.Join(original, DisableForCore(changed[i]))
			}
		}
		if len(changed) != 0 {
			original = errors.Join(original, restartGatewayPanel(recovery))
		}
		if before.Configured {
			original = errors.Join(original, network.Enable(recovery, before.Config))
		}
		return original
	}
	if action == "disable" {
		if err := network.Suspend(ctx); err != nil {
			return err
		}
		for _, name := range []string{service.CoreTypeXray, service.CoreTypeSingBox} {
			if !states[name].Enabled {
				continue
			}
			changed = append(changed, name)
			if err := DisableForCore(name); err != nil {
				return rollback(true, err)
			}
		}
		if len(changed) != 0 {
			if err := restartGatewayPanel(ctx); err != nil {
				return rollback(true, err)
			}
		}
		if err := network.Disable(ctx); err != nil {
			return rollback(true, err)
		}
		fmt.Println("Gateway Mode disabled")
		return nil
	}
	if before.Configured {
		cfg = before.Config
	}
	if err := network.Validate(ctx, cfg); err != nil {
		return err
	}
	other := service.CoreTypeSingBox
	if core == other {
		other = service.CoreTypeXray
	}
	if states[other].Enabled {
		return fmt.Errorf("Gateway belongs to %s; disable it before enabling %s", other, core)
	}
	if states[core].Enabled && !states[core].Configured {
		return fmt.Errorf("Gateway template recovery required; disable Gateway first")
	}
	if !states[core].Configured {
		if err := EnableForCore(core); err != nil {
			return err
		}
		changed = append(changed, core)
		if err := restartGatewayPanel(ctx); err != nil {
			return rollback(false, err)
		}
	}
	if err := network.Enable(ctx, cfg); err != nil {
		// Keep a working listener while any failed network cleanup still needs recovery.
		if network.Status(ctx).Configured && !before.Configured {
			return err
		}
		return rollback(false, err)
	}
	fmt.Println("Gateway Mode enabled (" + core + ")")
	return nil
}
func restartGatewayPanel(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "x-ui.service")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restart panel: %w: %s", err, output)
	}
	return nil
}
