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

const (
	ChunkSize      = 1400
	MaxRetries     = 3
	TimeoutSeconds = 30
)

type FileChunk struct {
	Index    uint32 // チャンクインデックス
	Length   uint32 // チャンクの長さ
	Checksum uint32 // チャンクのチェックサム（CRC32）
	Data     []byte // チャンク
}

// File index information
type FileIndexData struct {
	FilePath   string `json:"filepath"`
	TotalSize  int64  `json:"total_size"`
	ChunkCount uint32 `json:"chunk_count"`
	FileHash   string `json:"file_hash"`
	ChunkSize  int    `json:"chunk_size"`
}

type MissingPacketData struct {
	MissingChunks []uint32 `json:"missing_chunks"`
}

// Finish packet
type FinishPacketData struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

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
