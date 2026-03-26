package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set from main via SetVersion
var version = "dev"

func SetVersion(v string) {
	version = v
}

func GetVersion() string {
	return version
}

var checkUpdate bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Affiche la version de hop",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hop %s\n", version)

		if checkUpdate {
			latest, err := fetchLatestVersion()
			if err != nil {
				fmt.Printf("Impossible de verifier les mises a jour: %v\n", err)
				return
			}
			if latest != version && latest != "v"+version {
				fmt.Printf("Nouvelle version disponible: %s\n", latest)
				fmt.Println("https://github.com/meumeu-dev/hop/releases/latest")
			} else {
				fmt.Println("Vous etes a jour.")
			}
		}
	},
}

func init() {
	versionCmd.Flags().BoolVar(&checkUpdate, "check", false, "Verifie si une mise a jour est disponible")
	rootCmd.AddCommand(versionCmd)
}
