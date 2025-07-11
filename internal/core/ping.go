package core

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	timer      *time.Timer
	timeoutDur = 30 * time.Second
	mu         sync.Mutex
)

func (h *Handle) Ping() {
	for {
		Write(h.Self.Conn, h.Peer.Addr.StrAddr(), &BaseData{Type: Ping})
		logrus.Info("do ping!")

		time.Sleep(5 * time.Second)
	}
}

func warn() {
	logrus.Warn("No ping received. Please check your internet connection.")
}

func RecordPingTime() {
	mu.Lock()
	defer mu.Unlock()

	resetTimer()
	logrus.Info("get ping!")
}

func resetTimer() {
	if timer != nil {
		timer.Stop()
	}

	timer = time.AfterFunc(timeoutDur, warn)
}
