package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
)

func Client() (*Handle, error) {
	// トークン使用側（クライアント側）
	tty, err := utils.UseTty()
	if err != nil {
		return nil, err
	}

	cfg := &PeerConfig{}
	for {
		fmt.Print("Enter token: ")
		token, err := tty.ReadString()
		if err != nil {
			return nil, err
		}

		cfg, err = ParseToken(token)
		if err != nil {
			logrus.Info("\ninvaid token")
			continue
		}

		break
	}

	fmt.Println("Connecting to:", cfg.Name, cfg.Addr.Ip, cfg.Addr.Port)

	self, err := SetupPort()
	if err != nil {
		return nil, err
	}

	// 接続試行
	peer, err := Sync(self, cfg)
	if err != nil {
		return nil, err
	}

	if peer == nil {
		return nil, err
	}
	logrus.Info("Connected to:", peer.Name)

	// まず相手のトレイを受信
	logrus.Info("Waiting for peer's tray...")
	peertray, err := ReceiveTray(self, peer)
	if err != nil {
		return nil, err
	}

	// 自分のトレイを送信
	logrus.Info("Sending tray items...")
	err = TraySync(self, peer, tray.UseTray())
	if err != nil {
		return nil, err
	}
	logrus.Info("Tray sent successfully")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "filename\tsize\thash\n")
	for _, t := range *peertray {
		fmt.Fprintf(w, "%s\t%d\t%s\n", t.Filename, t.Size, t.Hash)
	}
	w.Flush()

	return &Handle{Self: self, Peer: peer}, nil
}
