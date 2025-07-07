package main

import (
	"fmt"

	"QuickPort/core"
	"QuickPort/shell"
	"QuickPort/tray"
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
		err := tray.SetTray("../tray/")
		if err != nil {
			logrus.Error(err)
			return
		}
		handle, err = core.Host()
		if err != nil {
			logrus.Error(err)
			return
		}

	case utils.UseToken:
		err := tray.SetTray("../tray2/")
		if err != nil {
			logrus.Error(err)
			return
		}

		for {
			handle, err = core.Client()
			if err != nil {
				logrus.Error(err)
				logrus.Info("Restart Setup")
				continue
			}

			break
		}

	case utils.DebugLevel:
		// デバッグモード
		logrus.Info("Debug mode selected")
	}

	fmt.Printf("%s:%d <==> %s:%d\n", handle.Self.Addr.Ip.String(), handle.Self.Addr.Port, handle.Peer.Addr.Ip.String(), handle.Peer.Addr.Port)

	//main Receiver
	/*/
	exit := make(chan bool, 1)
	recovery := make(chan bool, 1)
	go handle.Receiver(exit)

	go func() {
		for r := range recovery {
			if r {
				go handle.Receiver(exit)
			}
		}
	}()
	/*/

	//debug
	/*/
	buf := make(chan []byte, 1000)
	handle.NewReceiver(buf)
	/*/

	go handle.Receiver()

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
