package main

import (
	"os"

	"github.com/yaronhod/jira-board-keeper/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
