package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"encoding/json"
	"fmt"
	"net"
	"strconv"

	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

func GetLocalAddr() (*Address, error) {
	// 利用可能なネットワークインターフェースを取得
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// ループバックや無効なインターフェースをスキップ
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// IPv4のみを対象とし、プライベートIPアドレスを優先
			if ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
				// プライベートIPアドレスかチェック
				if isPrivateIP(ipNet.IP) {
					return &Address{
						Ip:   ipNet.IP,
						Port: utils.GetPort(),
					}, nil
				}
			}
		}
	}

	// プライベートIPが見つからない場合は、外部接続で取得
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return &Address{
		Ip:   conn.LocalAddr().(*net.UDPAddr).IP,
		Port: utils.GetPort(),
	}, nil
}

func GetLocalIPAlternative() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				// IPv4アドレスで、プライベートアドレス範囲をチェック
				if isPrivateIP(ipnet.IP) {
					return ipnet.IP, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no suitable local IP address found")
}
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"192.168.0.0/16", // 192.168.0.0 - 192.168.255.255
	}

	for _, cidr := range privateRanges {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func PortSetUp() (*SelfCfg, error) {
	self := SelfCfg{}

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
		// 代替方法を試す
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
func Sync(self *SelfCfg, peer *PeerCfg) (*PeerCfg, error) {
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
func SyncListener(self *SelfCfg) (*PeerCfg, error) {
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

			peer := &PeerCfg{
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

func TraySync(self *SelfCfg, peer *PeerCfg, defaultTray string) error {
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

func receiveFromPeer(self *SelfCfg, peer *PeerCfg) (*BaseData, error) {
	buf := make([]byte, 1024)

	for {
		n, peerAddr, err := self.Conn.ReadFromUDP(buf)
		if err != nil {
			if n == 0 && peerAddr == nil {
				continue
			}

			return nil, fmt.Errorf("receiver error")
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

func (a *Address) StrAddr() string {
	return a.Ip.String() + ":" + strconv.Itoa(a.Port)
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

func ConvertMapToFileReqMeta(input any) (*FileReqData, error) {
	// input が map[string]any だと仮定
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta FileReqData
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
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

// Helper functions for data conversion
func convertMapToMissingPacketData(input interface{}) (*MissingPacketData, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data MissingPacketData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func convertMapToFinishPacketData(input interface{}) (*FinishPacketData, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data FinishPacketData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
