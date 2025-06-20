package core

import (
	"context"
	"net"
	"sync"
)

type dataType int
type ReceiverMode int

const (
	SyncTray dataType = iota
	Auth
	Message
	FileReqest
	FileIndex
	File
)

const (
	FileJSON ReceiverMode = iota
	FileTransfer
	Receiver
)

const (
	ChunkSize      = 1400
	MaxRetries     = 3
	TimeoutSeconds = 30
)

type ReceiverController struct {
	Cancel context.CancelFunc
	Active bool
	Mu     sync.Mutex
}

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

type fileRequestData struct {
	FilePath string
}

type Address struct {
	Ip   net.IP
	Port int
}
type Handle struct {
	Self *SelfConfig
	Peer *PeerConfig
}
