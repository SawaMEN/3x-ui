package service

import "path/filepath"

func GatewayMigrationFiles() []string {
	paths := (&GatewayNetworkService{}).networkPaths()
	return []string{paths.state, paths.rpFilter, paths.nft, paths.routingUnit, paths.firewallUnit, paths.restoreScript}
}
func GatewayMigrationStatePath() string {
	path := (&GatewayNetworkService{}).networkPaths().state
	absolute, _ := filepath.Abs(path)
	return absolute
}
