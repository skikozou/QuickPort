package core

import "net"

type dataType int
type Event int

const (
	SyncTray dataType = iota
	Auth
	Message
	FileReqest
	FileIndex
	File
)

const (
	InvokeShell Event = iota
)

type ShellArgs struct {
	Arg    []string
	Handle *Handle
}

type BaseData struct {
	Type dataType
	Data interface{}
}

type FileReqData struct {
	FilePath string
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
