package screens

import (
	"github.com/fluid-movement/log-tui/config"
	gossh "github.com/fluid-movement/log-tui/ssh"
)

// ScreenID identifies which screen to navigate to.
type ScreenID int

const (
	ScreenProjects ScreenID = iota
	ScreenCreator
	ScreenFileList
	ScreenGrid
)

// SwitchMsg is produced by screen models and consumed by AppModel.
type SwitchMsg struct {
	To      ScreenID
	Payload any
}

// GridPayload carries data handed off from FileList to Grid.
type GridPayload struct {
	Project  config.Project
	FilePath string
	Clients  []*gossh.Client
}
