package main

import (
	"os"

	"github.com/fholzer/cloudflarebeat/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
