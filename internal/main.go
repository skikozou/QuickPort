package main

import (
	"fmt"

	"QuickPort/core"
	"QuickPort/utils"

	"github.com/sirupsen/logrus"
)

func SelectMode() (utils.StartUpMode, error) {
	//select mode
	fmt.Println("Use token - 1\nGen token - 2")

	for {
		tty, err := utils.UseTty()
		if err != nil {
			logrus.Fatal(err)
			return utils.DebugLevel, err
		}

		mode, err := tty.ReadString()
		if err != nil {
			logrus.Fatal(err)
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
	/*/
	/*/

	utils.SetUpLogrus()
	utils.OpenTty()

	mode, err := SelectMode()
	if err != nil {
		logrus.Fatal(err)
		return
	}

	switch mode {
	case utils.GenToken:
		self, err := core.PortSetUp()
		if err != nil {
			logrus.Fatal(err)
			return
		}

		token := core.GenToken(self)
		logrus.Info(fmt.Sprintf("Your token: %s", token))

		peer, err := core.SyncListener(self)
		if err != nil {
			logrus.Error(err)
			return
		}

		logrus.Info(*peer)

		tray, err := core.TrayRecieve(self, peer)
		if err != nil {
			logrus.Error(err)
			return
		}

		logrus.Info(*tray)

		logrus.Info("do sync")
		err = core.TraySync(self, peer, `C:\Users\skiko\go\QuickPort\tray`)
		if err != nil {
			logrus.Error(err)
			return
		}
	case utils.UseToken:
		tty, err := utils.UseTty()
		if err != nil {
			logrus.Error(err)
			return
		}

		token, err := tty.ReadString()
		if err != nil {
			logrus.Error(err)
			return
		}

		data, err := core.ParseToken(token)
		if err != nil {
			logrus.Error(err)
			return
		}

		fmt.Println(data.Name, data.Addr.Ip, data.Addr.Port)

		self, err := core.PortSetUp()
		if err != nil {
			logrus.Fatal(err)
			return
		}

		peer, err := core.Sync(self, data)
		if err != nil {
			logrus.Error(err)
			return
		}

		if peer == nil {
			logrus.Error(err)
			return
		}

		logrus.Info(*peer)

		logrus.Info("do sync")
		err = core.TraySync(self, peer, `C:\Users\skiko\go\QuickPort\tray2`)
		if err != nil {
			logrus.Error(err)
			return
		}

		tray, err := core.TrayRecieve(self, peer)
		if err != nil {
			logrus.Error(err)
			return
		}

		logrus.Info(*tray)

	case utils.DebugLevel:

	}
	/*/

	fmt.Print("Enter peer address (ip:port): ")
	var peer string
	fmt.Scanln(&peer)

	go core.ReceiveLoop(conn)

	//tray share
	defaultTray := `C:\Users\skiko\go\QuickPort\tray`
	items, err := tray.GetTrayItems(defaultTray)
	if err != nil {
		return
	}

	core.Write(conn, peer, &core.BaseData{
		Type: core.TraySync,
		Data: items,
	})

	var wait string
	fmt.Scanln(&wait)
	/*/
}
