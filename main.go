package main

import "github.com/meumeu-dev/hop/cmd"

func main() {
	cmd.SetVersion(Version)
	cmd.Execute()
}
