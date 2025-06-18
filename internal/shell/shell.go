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
			return handle, err
		}
		args := core.ShellArgs{
			Arg:    strings.Split(cmd, " "),
			Handle: handle,
		}

		switch args.Head() {
		case "get":
			err := core.GetFile(handle, args.Next())
			if err != nil {
				return handle, err
			}
		case "exit":
			return handle, nil
			//exit process
		default:
			fmt.Printf("invaid command :%s\n", args.Head())
		}
	}
}
