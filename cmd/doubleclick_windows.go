//go:build windows

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// isDoubleClicked returns true when hop.exe is the only process attached to
// its console, which happens when the user launched it from Explorer.
// In that case the console window closes the instant main() returns, so we
// keep it open with a prompt.
func isDoubleClicked() bool {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetConsoleProcessList")
	var pids [4]uint32
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return r == 1
}

func init() {
	if !isDoubleClicked() {
		return
	}
	fmt.Println("hop est un outil en ligne de commande.")
	fmt.Println()
	fmt.Println("Ouvre PowerShell puis:")
	fmt.Println("  cd \"$env:USERPROFILE\\Downloads\"")
	fmt.Println("  .\\hop.exe config")
	fmt.Println()
	fmt.Println("Ou installe-le proprement:")
	fmt.Println("  iwr -useb meumeu.dev/hop/install.ps1 | iex")
	fmt.Println()
	fmt.Print("Appuie sur Entree pour fermer...")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(0)
}
