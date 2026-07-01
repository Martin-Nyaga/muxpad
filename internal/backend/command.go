package backend

import (
	"strings"

	"github.com/Martin-Nyaga/muxpad/internal/config"
	"github.com/Martin-Nyaga/muxpad/internal/shellwords"
)

type CommandWrapOptions struct {
	CloseCommand  string
	FinishCommand string
}

func WrappedCommand(command string, exitMode config.ExitMode, opts CommandWrapOptions) string {
	if exitMode == "" {
		exitMode = config.ExitKeepOnError
	}
	closeCommand := opts.CloseCommand
	if strings.TrimSpace(closeCommand) == "" {
		closeCommand = `exit "$status"`
	}
	if exitMode == config.ExitClose {
		inner := "muxpad_command=" + shellwords.Escape(command) + "\n" +
			"( " + command + "\n); status=$?\n" +
			closeCommand
		return "sh -c " + shellwords.Escape(inner)
	}
	finish := ""
	if strings.TrimSpace(opts.FinishCommand) != "" {
		finish = "\n  " + opts.FinishCommand
	}
	drop := `muxpad_seed_history() {
  [ -n "$muxpad_command" ] || return 0
  case "${SHELL##*/}" in
    zsh) printf ': %s:0;%s\n' "$(date +%s 2>/dev/null || echo 0)" "$muxpad_command" >> "${HISTFILE:-$HOME/.zsh_history}" 2>/dev/null ;;
    bash) printf '%s\n' "$muxpad_command" >> "${HISTFILE:-$HOME/.bash_history}" 2>/dev/null ;;
  esac
}
muxpad_drop() {` + finish + `
  muxpad_seed_history
  exec "${SHELL:-/bin/sh}"
}
trap 'status=$?; muxpad_drop' INT TERM`
	tail := "muxpad_drop"
	if exitMode == config.ExitKeepOnError {
		tail = `if [ $status -eq 0 ]; then ` + closeCommand + `; else muxpad_drop; fi`
	}
	inner := "muxpad_command=" + shellwords.Escape(command) + "\n" +
		drop + "\n" +
		"( " + command + "\n); status=$?\n" +
		tail
	return "sh -c " + shellwords.Escape(inner)
}
