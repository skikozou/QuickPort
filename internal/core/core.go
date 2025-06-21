package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"
)

func SetupPort() (*SelfConfig, error) {
	self := SelfConfig{}

	fmt.Printf("Enter your name: ")
	tty, err := utils.UseTty()
	if err != nil {
		return nil, err
	}

	self.Name, err = tty.ReadString()
	if err != nil {
		return nil, err
	}

	self.LocalAddr, err = GetLocalAddr()
	if err != nil {
		logrus.Warn("Primary IP detection failed, trying alternative method")
		ip, altErr := GetLocalIPAlternative()
		if altErr != nil {
			return nil, fmt.Errorf("failed to get local IP: %v (alternative: %v)", err, altErr)
		}
		self.LocalAddr = &Address{
			Ip:   ip,
			Port: utils.GetPort(),
		}
	}

	return &self, err
}

// Sync 関数を改善
func Sync(self *SelfConfig, peer *PeerConfig) (*PeerConfig, error) {
	port := utils.GetPort()
	conn, err := ListenUDP(port)
	if err != nil {
		return nil, err
	}

	self.Conn = conn
	logrus.Infof("Listening on %s:%d", self.LocalAddr.Ip.String(), port)

	// 認証リクエスト送信
	addr := fmt.Sprintf("%s:%d", peer.Addr.Ip.String(), peer.Addr.Port)
	logrus.Debug("Sending auth request to:", addr)

	err = Write(conn, addr, &BaseData{
		Type: Auth,
		Data: tray.AuthMeta{Name: self.Name, Flag: tray.AccessReq},
	})
	if err != nil {
		logrus.Error("Failed to send auth request:", err)
		return nil, err
	}

	// 認証レスポンス受信
	logrus.Debug("Waiting for auth response...")
	meta, err := receiveFromPeer(self, peer)
	if err != nil {
		logrus.Error("Failed to receive auth response:", err)
		return nil, err
	}

	if meta.Type != Auth {
		logrus.Error("Invalid response type")
		return nil, fmt.Errorf("invalid response type: %d", meta.Type)
	}

	authmeta, err := ConvertMapToAuthMeta(meta.Data)
	if err != nil {
		logrus.Error("Failed to parse auth meta:", err)
		return nil, err
	}

	switch authmeta.Flag {
	case tray.AccessReq:
		return nil, fmt.Errorf("invalid packet - received request instead of response")
	case tray.Allow:
		logrus.Info("Connection accepted!")
	case tray.Deny:
		logrus.Info("Connection denied by peer")
		return nil, nil
	}

	return peer, nil
}

// SyncListener 関数を改善
func SyncListener(self *SelfConfig) (*PeerConfig, error) {
	port := utils.GetPort()
	conn, err := ListenUDP(port)
	if err != nil {
		return nil, err
	}

	self.Conn = conn
	self.LocalAddr.Port = port

	logrus.Infof("Listening on %s:%d", self.LocalAddr.Ip.String(), port)

	buf := make([]byte, 1024)
waitPeer:
	for {
		n, peerAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			logrus.Error("UDP read error:", err)
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Error("JSON unmarshal error:", err)
			continue
		}

		if meta.Type != Auth {
			logrus.Debug("Ignoring non-auth packet")
			continue
		}

		authmeta, err := ConvertMapToAuthMeta(meta.Data)
		if err != nil {
			logrus.Error("Failed to parse auth meta:", err)
			continue
		}

		if authmeta.Flag != tray.AccessReq {
			logrus.Debug("Ignoring non-request auth packet")
			continue
		}

		tty, err := utils.UseTty()
		if err != nil {
			return nil, err
		}

		for {
			fmt.Printf("%s (%s:%d) is requesting to connect. Accept? (y/n)\n>",
				authmeta.Name, peerAddr.IP.String(), peerAddr.Port)
			answer, err := tty.ReadString()
			if err != nil {
				return nil, err
			}

			peer := &PeerConfig{
				Name: authmeta.Name,
				Addr: &Address{
					Ip:   peerAddr.IP,
					Port: peerAddr.Port,
				},
			}

			switch answer {
			case "y":
				// 承認レスポンス送信
				err = Write(conn, fmt.Sprintf("%s:%d", peerAddr.IP.String(), peerAddr.Port),
					&BaseData{Type: Auth, Data: tray.AuthMeta{Name: self.Name, Flag: tray.Allow}})
				if err != nil {
					logrus.Error("Failed to send allow response:", err)
					return nil, err
				}

				logrus.Info("Connection accepted!")
				return peer, nil

			case "n":
				// 拒否レスポンス送信
				err = Write(conn, fmt.Sprintf("%s:%d", peerAddr.IP.String(), peerAddr.Port),
					&BaseData{Type: Auth, Data: tray.AuthMeta{Name: self.Name, Flag: tray.Deny}})
				if err != nil {
					logrus.Error("Failed to send deny response:", err)
				}

				logrus.Info("Connection denied, waiting for other peer...")
				continue waitPeer

			default:
				fmt.Println("Please enter 'y' or 'n'")
			}
		}
	}
}

func TraySync(self *SelfConfig, peer *PeerConfig, defaultTray string) error {
	items, err := tray.GetTrayItems(defaultTray)
	if err != nil {
		return err
	}

	err = Write(self.Conn, fmt.Sprintf("%s:%d", peer.Addr.Ip.String(), peer.Addr.Port), &BaseData{
		Type: SyncTray,
		Data: items,
	})

	if err != nil {
		return err
	}

	return nil
}

func (a *Address) StrAddr() string {
	return a.Ip.String() + ":" + strconv.Itoa(a.Port)
}
