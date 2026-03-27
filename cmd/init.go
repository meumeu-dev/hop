package cmd

import (
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:    "init",
	Short:  "Configure hop (alias de hop config)",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		configCmd.Run(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
