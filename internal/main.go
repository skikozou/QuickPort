package main

import (
	"fmt"

	"QuickPort/core"
	"QuickPort/shell"
	"QuickPort/utils"

	"github.com/sirupsen/logrus"
)

func SelectMode() (utils.StartUpMode, error) {
	//select mode
	fmt.Println("Use token - 1\nGen token - 2")

	for {
		tty, err := utils.UseTty()
		if err != nil {
			return utils.DebugLevel, err
		}

		mode, err := tty.ReadString()
		if err != nil {
			return utils.DebugLevel, nil
		}

		switch mode {
		case "1":
			return utils.UseToken, nil
		case "2":
			return utils.GenToken, nil
		case "dev":
			return utils.DebugLevel, nil
		default:
			//return first
		}
	}
}

func main() {
	utils.SetUpLogrus()
	utils.OpenTty()

	mode, err := SelectMode()
	if err != nil {
		logrus.Fatal(err)
		return
	}

	var handle *core.Handle
	switch mode {
	case utils.GenToken:
		handle, err = core.Host()
		if err != nil {
			logrus.Error(err)
			return
		}

	case utils.UseToken:
		handle, err = core.Client()
		if err != nil {
			logrus.Error(err)
			return
		}

	case utils.DebugLevel:
		// デバッグモード
		logrus.Info("Debug mode selected")
	}

	fmt.Printf("%s:%d <==> %s:%d\n", handle.Self.LocalAddr.Ip.String(), handle.Self.LocalAddr.Port, handle.Peer.Addr.Ip.String(), handle.Peer.Addr.Port)

	//main reciever
	go core.Reciever(handle)

	//shell
	handle, err = shell.Run(handle)
	if err != nil {
		logrus.Error(err)
		return
	}

	if handle == nil {
		logrus.Info("Process exit")
		return
	}
}
