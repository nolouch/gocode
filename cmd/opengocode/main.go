package main

import (
	"os"

	"github.com/nolouch/opengocode/internal/cli/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
