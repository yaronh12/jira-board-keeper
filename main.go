package main

import (
	"os"

	"github.com/yaronhod/jira-board-reporter/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
