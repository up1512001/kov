// Package main provides the entry point for the kov CLI.
// Kov is an indestructible AI coding CLI built in Go.
// https://trykov.dev
package main

import (
	"os"

	"github.com/utsavkovy/kov/internal/app"
)

// Version information — injected at build time by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	a := app.New(app.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})

	if err := a.RootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
