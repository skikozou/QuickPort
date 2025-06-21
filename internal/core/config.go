package core

import "net"

type PeerConfig struct {
	Name string
	Addr *Address
}

type SelfConfig struct {
	Name       string
	Conn       *net.UDPConn
	GlobalAddr *Address
	LocalAddr  *Address
}
