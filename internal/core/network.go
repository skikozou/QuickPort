package core

import (
	"QuickPort/utils"
	"encoding/json"
	"fmt"
	"net"

	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

func receiveFromPeer(self *SelfConfig, peer *PeerConfig, useSub bool) (*BaseData, error) {
	buf := make([]byte, 1024)
	conn := self.Conn
	if useSub {
		conn = self.SubConn
	}

	for {
		n, peerAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if n == 0 && peerAddr == nil {
				continue
			}

			logrus.Debugf("fuckin packet: %d %s", n, err.Error())

			return nil, fmt.Errorf("receiver error")
		}

		if utils.DebugVal["secondShare"] == "1" {
			logrus.Debug(peerAddr.IP, peerAddr.Port, n)
		}

		if useSub {
			if peerAddr.IP.String() != peer.SubAddr.Ip.String() || peerAddr.Port != peer.SubAddr.Port {
				continue
			}
		} else {
			if peerAddr.IP.String() != peer.Addr.Ip.String() || peerAddr.Port != peer.Addr.Port {
				continue
			}
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
