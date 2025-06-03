package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

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

	// 1. STUNで外部IPとポート取得
	addr, err := GetExternalAddress()
	if err != nil {
		log.Fatalln("STUN error:", err)
	}

	self.GlobalAddr = &Address{
		Ip:   addr.IP,
		Port: addr.Port,
	}

	logrus.Printf("Your external address: %s:%d\n", addr.IP.String(), addr.Port)

	return &self, err
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
	logrus.Infof("Listening on %s", conn.LocalAddr().String())
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

		if meta.Type != TraySync {
			continue
		}

		Read(&meta)

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

		Read(&meta)
	}
}

func Read(data *BaseData) {
	fmt.Println(data.Data)
	switch data.Type {
	case TraySync:
		meta, err := ConvertMapToFileMeta(data.Data)
		if err != nil {
			logrus.Fatal("型エラー")
		}

		data.Data = meta
	case File:

	case Message:

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
