//go:build ignore

package main

import (
	"fmt"
	"github.com/meumeu-dev/hop/internal/account"
	"github.com/meumeu-dev/hop/internal/config"
)

func main() {
	config.Init()
	client := account.NewClient("https://hop-pair.meumeudev.workers.dev")
	session, err := client.Register("contact@meumeu.dev", "meumeu", "tgboulet")
	if err != nil {
		fmt.Printf("Register: %v\n", err)
		return
	}
	fmt.Printf("OK! %s (%s)\n", session.Username, session.AccountID)
	account.SaveSession(session)
	fmt.Println("Session saved!")
}
