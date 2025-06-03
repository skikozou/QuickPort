package utils

import "github.com/mattn/go-tty"

var ttyHandler tty.TTY

const (
	GenToken StartUpMode = iota
	UseToken
	DebugLevel
)

const (
	BasePort int = 55190
	MaxPort  int = 55199
)

type StartUpMode int
