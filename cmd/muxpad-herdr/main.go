package main

import (
	"fmt"
	"os"

	"github.com/Martin-Nyaga/muxpad/internal/app"
	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/herdr"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr *os.File) int {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "open-palette":
		if err := herdr.New().OpenPalette(); err != nil {
			fmt.Fprintf(stderr, "muxpad-herdr: %v\n", err)
			return 1
		}
		return 0
	case "", "palette":
		if err := runPalette(); err != nil {
			fmt.Fprintf(stderr, "muxpad-herdr: %v\n", err)
			return 1
		}
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stderr, "Usage: muxpad-herdr open-palette|palette\n")
		return 0
	default:
		fmt.Fprintf(stderr, "muxpad-herdr: unknown command: %s\n", command)
		return 1
	}
}

func runPalette() error {
	cfg, err := config.LoadHerdr()
	if err != nil {
		return err
	}
	return app.New(cfg, herdr.New()).DeclaredTaskMenu()
}
