package shell

import (
	"QuickPort/core"
	"QuickPort/utils"
	"fmt"
	"strings"
)

func Run(handle *core.Handle) (*core.Handle, error) {
	tty, err := utils.UseTty()
	if err != nil {
		return nil, err
	}

	for {
		fmt.Printf("> ")
		cmd, err := tty.ReadString()
		if err != nil {
			return nil, err
		}
		args := ShellArgs{
			Arg:    strings.Split(cmd, " "),
			handle: handle,
		}

		switch args.Head() {
		case "send":
			//send file
		case "exit":
			return nil, nil
			//exit process
		default:
			fmt.Printf("invaid command :%s\n", args.Head())
		}
	}
}

func (a *ShellArgs) Next() *ShellArgs {
	a.Arg = a.Arg[1:]
	return a
}

func (a *ShellArgs) Head() string {
	return a.Arg[0]
}

func (a *ShellArgs) Len() int {
	return len(a.Arg)
}
