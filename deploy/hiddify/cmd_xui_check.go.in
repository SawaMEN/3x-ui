package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	command := &cobra.Command{
		Use:   "check",
		Short: "Validate raw server configuration without starting listeners",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(configPaths) == 0 && len(configDirectories) == 0 {
				return fmt.Errorf("check requires --config or --config-directory")
			}
			return check()
		},
	}
	command.Flags().StringArrayVarP(&configPaths, "config", "c", nil, "set configuration file path")
	command.Flags().StringArrayVarP(&configDirectories, "config-directory", "C", nil, "set configuration directory path")
	mainCommand.AddCommand(command)
}
