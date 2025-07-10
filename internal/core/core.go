package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

func (h *Handle) ResetConn() error {
	h.Self.Conn.Close()
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%s", strconv.Itoa(h.Self.Addr.Port)))
	if err != nil {
		return err
	}
	h.Self.Conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	h.Self.SubConn.Close()
	addr, err = net.ResolveUDPAddr("udp", fmt.Sprintf(":%s", strconv.Itoa(h.Self.SubAddr.Port)))
	if err != nil {
		return err
	}
	h.Self.SubConn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	if h.Self.Conn != nil {
		h.Self.Conn.SetReadDeadline(time.Time{})
	}
	if h.Self.SubConn != nil {
		h.Self.SubConn.SetReadDeadline(time.Time{})
	}

	return nil
}

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

	self.Addr, err = GetLocalAddr()
	if err != nil {
		logrus.Warn("Primary IP detection failed, trying alternative method")
		ip, altErr := GetLocalIPAlternative()
		if altErr != nil {
			return nil, fmt.Errorf("failed to get local IP: %v (alternative: %v)", err, altErr)
		}
		self.Addr = &Address{
			Ip:   ip,
			Port: utils.GetPort(),
		}
	}

	self.Conn, err = ListenUDP(self.Addr.Port)
	if err != nil {
		return nil, err
	}

	self.SubAddr = &Address{
		Ip:   self.Addr.Ip,
		Port: utils.GetPort(),
	}
	self.SubConn, err = ListenUDP(self.SubAddr.Port)
	if err != nil {
		return nil, err
	}

	return &self, err
}

// Sync 関数を改善
func Sync(self *SelfConfig, peer *PeerConfig) (*PeerConfig, error) {
	logrus.Infof("Listening on %s:%d", self.Addr.Ip.String(), self.Addr.Port)

	// 認証リクエスト送信
	addr := fmt.Sprintf("%s:%d", peer.Addr.Ip.String(), peer.Addr.Port)
	logrus.Debug("Sending auth request to:", addr)

	err := Write(self.Conn, addr, &BaseData{
		Type: Auth,
		Data: tray.AuthMeta{Name: self.Name, SubPort: self.SubAddr.Port, Flag: tray.AccessReq},
	})
	if err != nil {
		logrus.Error("Failed to send auth request:", err)
		return nil, err
	}

	// 認証レスポンス受信
	logrus.Debug("Waiting for auth response...")
	meta, err := receiveFromPeer(self, peer, false)
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
		peer.SubAddr = &Address{
			Ip:   peer.Addr.Ip,
			Port: authmeta.SubPort,
		}
	case tray.Deny:
		logrus.Info("Connection denied by peer")
		return nil, nil
	}

	return peer, nil
}

// SyncListener 関数を改善
func SyncListener(self *SelfConfig) (*PeerConfig, error) {
	logrus.Infof("Listening on %s:%d", self.Addr.Ip.String(), self.Addr.Port)

	buf := make([]byte, 1024)
waitPeer:
	for {
		n, peerAddr, err := self.Conn.ReadFromUDP(buf)
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
				SubAddr: &Address{
					Ip:   peerAddr.IP,
					Port: authmeta.SubPort,
				},
			}

			switch answer {
			case "y":
				// 承認レスポンス送信
				err = Write(self.Conn, fmt.Sprintf("%s:%d", peerAddr.IP.String(), peerAddr.Port),
					&BaseData{Type: Auth, Data: tray.AuthMeta{Name: self.Name, SubPort: self.SubAddr.Port, Flag: tray.Allow}})
				if err != nil {
					logrus.Error("Failed to send allow response:", err)
					return nil, err
				}

				logrus.Info("Connection accepted!")
				return peer, nil

			case "n":
				// 拒否レスポンス送信
				err = Write(self.Conn, fmt.Sprintf("%s:%d", peerAddr.IP.String(), peerAddr.Port),
					&BaseData{Type: Auth, Data: tray.AuthMeta{Name: self.Name, SubPort: self.SubAddr.Port, Flag: tray.Deny}})
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
