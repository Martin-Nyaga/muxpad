package main

import (
	"os"

	"github.com/Martin-Nyaga/muxpad/internal/tmuxcli"
)

func main() {
	os.Exit(tmuxcli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
