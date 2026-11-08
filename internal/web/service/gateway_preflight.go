package service

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Preflight inspects the same host prerequisites as Enable, without touching
// core templates, sysctls, policy routes, files, or firewall rules. Enable
// still performs authoritative validation after acquiring the operation lock.
func (s *GatewayNetworkService) Preflight(ctx context.Context, cfg GatewayNetworkConfig) error {
	if err := s.Validate(ctx, cfg); err != nil {
		return err
	}
	normalized, _, err := normalizeGatewayNetworkConfig(cfg)
	if err != nil {
		return err
	}
	state, stateErr := s.loadGatewayNetworkState()
	if stateErr == nil {
		if state.Config != normalized {
			return fmt.Errorf("режим шлюза уже настроен для другого интерфейса или адреса; сначала выключите его")
		}
		return nil
	}
	if !os.IsNotExist(stateErr) {
		return fmt.Errorf("состояние восстановления шлюза повреждено: %w", stateErr)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rules, err := s.command(ctx, "ip", "-N", "-4", "rule", "show")
	if err != nil {
		return err
	}
	if gatewayTableInUse(rules) {
		return fmt.Errorf("таблица маршрутизации 100 уже используется")
	}
	routes, routeErr := s.command(ctx, "ip", "-4", "route", "show", "table", "100")
	if routeErr != nil && !strings.Contains(routes, "FIB table does not exist") {
		return routeErr
	}
	if routeErr == nil && strings.TrimSpace(routes) != "" {
		return fmt.Errorf("в таблице маршрутизации 100 уже есть маршруты")
	}
	tables, err := s.command(ctx, "nft", "list", "tables")
	if err != nil {
		return err
	}
	if gatewayTablePresent(tables, "inet", "xui_gateway") || gatewayTablePresent(tables, "ip", "xui_gateway_nat") {
		return fmt.Errorf("обнаружены таблицы firewall шлюза без сохранённого состояния; требуется проверка администратора")
	}
	if _, err := os.Stat(s.networkPaths().rpFilter); err == nil {
		return fmt.Errorf("найдено устаревшее сохранение rp_filter; требуется восстановление")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
