//go:build windows

package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/meumeu-dev/hop/internal/pairing"
	"github.com/spf13/cobra"
)

// Hidden sub-command invoked by the elevated helper: it receives the
// pub key base64-encoded on the command line and appends it to
// administrators_authorized_keys + fixes the ACLs. Not meant to be
// called by users directly.
var winAdminAuthCmd = &cobra.Command{
	Use:    "_win-admin-authkey <base64-pubkey>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		raw, err := base64.StdEncoding.DecodeString(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Erreur decode base64:", err)
			os.Exit(2)
		}
		pub := string(raw)
		if err := pairing.ValidateSSHPublicKey(pub); err != nil {
			fmt.Fprintln(os.Stderr, "Cle invalide:", err)
			os.Exit(3)
		}
		dir := `C:\ProgramData\ssh`
		os.MkdirAll(dir, 0755)
		path := filepath.Join(dir, "administrators_authorized_keys")
		if err := pairing.AppendKeyUniqueExported(path, pub); err != nil {
			fmt.Fprintln(os.Stderr, "Erreur ecriture:", err)
			os.Exit(4)
		}
		// Lock ACLs as OpenSSH Windows expects
		exec.Command("icacls", path, "/inheritance:r",
			"/grant", "Administrators:F", "/grant", "SYSTEM:F").Run()
		fmt.Println("OK")
	},
}

func init() {
	rootCmd.AddCommand(winAdminAuthCmd)
}
