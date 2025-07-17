package network

import (
	"net"
	"time"
)

type ConnectionConfig struct {
	LocalPort     int
	BufferSize    int
	Timeout       time.Duration
	MaxRetries    int
	RetryInterval time.Duration
}

type Connection struct {
	conn       *net.UDPConn
	config     ConnectionConfig
	remoteAddr *net.UDPAddr
	localAddr  *net.UDPAddr
	isHost     bool
}
