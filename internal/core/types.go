package core

import "net"

type dataType int

const (
	SyncTray dataType = iota
	Auth
	Message
	File
)

type BaseData struct {
	Type dataType
	Data interface{}
}

type Address struct {
	Ip   net.IP
	Port int
}

type PeerCfg struct {
	Name string
	Addr *Address
}

type SelfCfg struct {
	Name       string
	Conn       *net.UDPConn
	GlobalAddr *Address
	LocalAddr  *Address
}

type Handle struct {
	Self *SelfCfg
	Peer *PeerCfg
}
