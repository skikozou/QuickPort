package shell

import (
	"QuickPort/core"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
)

var myHandle *core.Handle

type state int

const (
	stateNormal state = iota
	stateModal
)

type model struct {
	width, height int

	state       state
	logs        []string
	input       textinput.Model
	viewport    viewport.Model
	modalChoice int
}
