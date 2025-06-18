package core

import (
	"QuickPort/tray"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
)

func Host() (*Handle, error) {
	// トークン生成側（サーバー側）
	self, err := PortSetUp()
	if err != nil {
		return nil, err
	}

	token := GenToken(self)
	logrus.Info(fmt.Sprintf("Your token: %s", token))

	// 接続待ち
	peer, err := SyncListener(self)
	if err != nil {
		return nil, err
	}
	logrus.Info("Peer connected:", peer.Name)

	// まず自分のトレイを送信
	logrus.Info("Sending tray items...")
	err = TraySync(self, peer, tray.UseTray())
	if err != nil {
		return nil, err
	}
	logrus.Info("Tray sent successfully")

	// 相手のトレイを受信
	logrus.Info("Waiting for peer's tray...")
	tray, err := TrayReceive(self, peer)
	if err != nil {
		return nil, err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "filename\tsize\thash\n")
	for _, t := range *tray {
		fmt.Fprintf(w, "%s\t%d\t%s\n", t.Filename, t.Size, t.Hash)
	}
	w.Flush()

	return &Handle{Self: self, Peer: peer}, nil
}
