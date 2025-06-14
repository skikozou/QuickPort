package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"encoding/json"
	"fmt"
	"net"

	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

func GetLocalAddr() (*Address, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// 自分側のアドレスを取得
	return &Address{
		Ip:   conn.LocalAddr().(*net.UDPAddr).IP,
		Port: utils.GetPort(),
	}, nil

}

func PortSetUp() (*Self, error) {
	self := Self{}

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

	return &self, err
}

func TraySync(self *Self, peer *Peer, defaultTray string) error {
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

func receiveFromPeer(self *Self, peer *Peer) (*BaseData, error) {
	buf := make([]byte, 1024)
	for {
		n, peerAddr, err := self.Conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}

		if peerAddr.IP.String() != peer.Addr.Ip.String() || peerAddr.Port != peer.Addr.Port {
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			return nil, err
		}

		return &meta, nil
	}
}

func TrayRecieve(self *Self, peer *Peer) (*[]tray.FileMeta, error) {
	meta, err := receiveFromPeer(self, peer)
	if err != nil {
		return nil, err
	}

	if meta.Type != SyncTray {
		return nil, fmt.Errorf("invaid packet")
	}

	return ConvertMapToFileMeta(meta.Data)

}

func Sync(self *Self, peer *Peer) (*Peer, error) {
	port := utils.GetPort()
	conn, err := ListenUDP(port)
	if err != nil {
		return nil, err
	}

	self.Conn = conn

	logrus.Infof("Listening on %s:%d", string(self.LocalAddr.Ip.String()), self.LocalAddr.Port)

	addr := fmt.Sprintf("%s:%d", peer.Addr.Ip.String(), peer.Addr.Port)
	err = Write(conn, addr, &BaseData{Type: Auth, Data: tray.AuthMeta{Name: self.Name, Flag: tray.AccessReq}})
	if err != nil {
		return nil, err
	}

	meta, err := receiveFromPeer(self, peer)
	if err != nil {
		return nil, err
	}

	if meta.Type != Auth {
		return nil, err
	}

	authmeta, err := ConvertMapToAuthMeta(meta)
	if err != nil {
		return nil, err
	}

	switch authmeta.Flag {
	case tray.AccessReq:
		return nil, fmt.Errorf("invaid packet")
	case tray.Allow:
		logrus.Info("Connected!")
	case tray.Deny:
		logrus.Info("Access denied")
		return nil, nil
	}

	return peer, nil
}

func Listener(self *Self) error {
	port := utils.GetPort()
	conn, err := ListenUDP(port)
	if err != nil {
		return err
	}

	self.Conn = conn
	self.LocalAddr = &Address{
		Ip:   []byte("127.0.0.1"),
		Port: port,
	}

	fmt.Printf("Your local address:    127.0.0.1:%d\n", port)

	return nil
}

func SyncListener(self *Self) (*Peer, error) {
	port := utils.GetPort()
	conn, err := ListenUDP(port)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Listening on %s:%d", string(self.LocalAddr.Ip.String()), self.LocalAddr.Port)

	buf := make([]byte, 1024)
waitPeer:
	for {
		n, peerAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			logrus.Error(err)
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Fatal(err)
			return nil, err
		}

		if meta.Type != Auth {
			continue
		}

		authmeta, err := ConvertMapToAuthMeta(meta.Data)
		if err != nil {
			return nil, err
		}

		tty, err := utils.UseTty()
		if err != nil {
			return nil, err
		}

		for {
			fmt.Printf("%s (%s:%d) is requesting to connect. Accept? (y/n)\n>", authmeta.Name, peerAddr.IP.String(), peerAddr.Port)
			answer, err := tty.ReadString()
			if err != nil {
				return nil, err
			}

			switch answer {
			case "y":
				logrus.Info("Connected!")
				self.Conn = conn
				return &Peer{
					Name: authmeta.Name,
					Addr: &Address{
						Ip:   peerAddr.IP,
						Port: peerAddr.Port,
					},
				}, nil
			case "n":
				logrus.Info("Wait other peer...")
				continue waitPeer
			default:
				//none
			}
		}
	}
}

// UDPポートをバインドして、リッスン状態にする
func ListenUDP(port int) (*net.UDPConn, error) {
	addr := net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// STUNを使って外部アドレスを取得
func GetExternalAddress() (*stun.XORMappedAddress, error) {
	conn, err := net.Dial("udp", "stun.l.google.com:19302")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	c, err := stun.NewClient(conn)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	var resultAddr *stun.XORMappedAddress

	err = c.Do(stun.MustBuild(stun.TransactionID, stun.BindingRequest), func(e stun.Event) {
		if e.Error != nil {
			err = e.Error
			return
		}
		addr := &stun.XORMappedAddress{}
		if parseErr := addr.GetFrom(e.Message); parseErr != nil {
			err = parseErr
			return
		}
		resultAddr = addr
	})
	if err != nil {
		return nil, err
	}

	return resultAddr, nil
}

// pingメッセージを送信
func SendPing(conn *net.UDPConn, targetAddr string) error {
	raddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP([]byte("ping"), raddr)
	return err
}

// data share
func Write(conn *net.UDPConn, targetAddr string, data *BaseData) error {
	raddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = conn.WriteToUDP(raw, raddr)
	return err
}

func ReceiveSync(conn *net.UDPConn) (*BaseData, error) {
	buf := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Fatal(err)
			return nil, err
		}

		if meta.Type != SyncTray {
			continue
		}

		//Read(&meta)
		//ConvertMapToFileMeta()

		return &meta, nil
	}
}

// 受信処理
func ReceiveLoop(conn *net.UDPConn) {
	buf := make([]byte, 1024*1024)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			logrus.Errorln("Read error:", err)
			continue
		}
		logrus.Info("Received from %s", addr.String())

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Fatal(err)
			return
		}

		ConvertMapToFileMeta(meta.Data)
	}
}

func ConvertMapToFileMeta(input any) (*[]tray.FileMeta, error) {
	// input が map[string]any だと仮定
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta []tray.FileMeta
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}
func ConvertMapToAuthMeta(input any) (*tray.AuthMeta, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta tray.AuthMeta
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}
