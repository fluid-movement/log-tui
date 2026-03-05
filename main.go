package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/clog"
	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/ui"
)

func main() {
	clog.Init()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(ui.New(cfg))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
