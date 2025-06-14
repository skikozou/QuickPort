package utils

import (
	"net"

	"github.com/mattn/go-tty"
	"github.com/sirupsen/logrus"
)

func GetPort() int {
	for i := BasePort; i < MaxPort; i++ {
		if CheckPort(i) {
			return i
		}
	}
	return 0
}

func CheckPort(port int) bool {
	udpAddr := net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}
	udpLn, err := net.ListenUDP("udp", &udpAddr)
	if err != nil {
		return false
	}
	udpLn.Close()
	return true
}

func SetUpLogrus() {
	//logrus setup
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:            true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})
	logrus.SetLevel(logrus.DebugLevel)
}

func OpenTty() (*tty.TTY, error) {
	Tty, err := tty.Open()
	ttyHandler = *Tty
	return &ttyHandler, err
}

func UseTty() (*tty.TTY, error) {
	return &ttyHandler, nil
}
