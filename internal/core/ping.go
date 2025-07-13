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
	errorLevel = 0
)

func (h *Handle) Ping() {
	isPause := false
	for {
		select {
		case p, ok := <-h.Pause:
			if ok {
				isPause = p
				continue
			}
		default:
			if isPause {
				continue
			}

			Write(h.Self.Conn, h.Peer.Addr.StrAddr(), &BaseData{Type: Ping})

			time.Sleep(5 * time.Second)
		}
	}
}

func warn() {
	switch errorLevel {
	case 0:
		logrus.Warn("No ping received. Trying recovery connection...")
		//reset receiver
	case 1:
	case 2:
	case 3:
	case 4:
	case 5:
	}
}

func RecordPingTime() {
	mu.Lock()
	defer mu.Unlock()

	resetTimer()
}

func resetTimer() {
	if timer != nil {
		timer.Stop()
	}

	timer = time.AfterFunc(timeoutDur, warn)
}
