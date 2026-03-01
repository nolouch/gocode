package main

import (
	"os"

	"github.com/nolouch/gocode/internal/cli/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
