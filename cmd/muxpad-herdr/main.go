package main

import (
	"fmt"
	"os"

	"github.com/Martin-Nyaga/muxpad/internal/backend"
	"github.com/Martin-Nyaga/muxpad/internal/herdr"
)

const hardcodedCommand = `printf 'muxpad herdr skeleton running in %s\n' "${SHELL:-/bin/sh}"; while :; do sleep 3600; done`

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr *os.File) int {
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "", "launch-hardcoded":
		if err := launchHardcoded(herdr.New()); err != nil {
			fmt.Fprintf(stderr, "muxpad-herdr: %v\n", err)
			return 1
		}
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stderr, "Usage: muxpad-herdr launch-hardcoded\n")
		return 0
	default:
		fmt.Fprintf(stderr, "muxpad-herdr: unknown command: %s\n", command)
		return 1
	}
}

func launchHardcoded(client backend.Backend) error {
	pane, err := client.CreateTab(backend.CreateTabSpec{
		Label:     "muxpad skeleton",
		Directory: os.Getenv("PWD"),
		Focus:     true,
	})
	if err != nil {
		return err
	}
	return client.RunInPane(pane, hardcodedCommand)
}
