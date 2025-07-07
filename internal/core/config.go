package core

import "net"

type PeerConfig struct {
	Name    string
	Addr    *Address
	SubAddr *Address
}

type SelfConfig struct {
	Name    string
	Conn    *net.UDPConn
	SubConn *net.UDPConn
	Addr    *Address
	SubAddr *Address
}
