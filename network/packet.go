package network

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	// パケットヘッダーサイズ
	PacketIDSize   = 8
	ChunkIndexSize = 4
	ChunkSizeSize  = 4
	ChecksumSize   = 4
	HeaderSize     = PacketIDSize + ChunkIndexSize + ChunkSizeSize + ChecksumSize
)

const (
	// 基本パケット種別
	PacketTypeControl uint8 = iota
	PacketTypeFile
	PacketTypeMessage

	// 制御パケットのサブタイプ
	PacketSubTypeConnectionRequest uint8 = iota
	PacketSubTypeConnectionResponse
	PacketSubTypeConnectionConfirm
	PacketSubTypeDisconnect
	PacketSubTypeKeepAlive
	PacketSubTypeChunkTest
	PacketSubTypeChunkResult
)

type PacketHeader struct {
	ID         uint64
	ChunkIndex int32
	ChunkSize  uint32
	Checksum   uint32
}

type Packet struct {
	Header  PacketHeader
	Payload []byte
}

func NewPacket(id uint64, chunkIndex int32, payload []byte) *Packet {
	packet := &Packet{
		Header: PacketHeader{
			ID:         id,
			ChunkIndex: chunkIndex,
			ChunkSize:  uint32(len(payload)),
		},
		Payload: payload,
	}

	// チェックサムの計算
	packet.Header.Checksum = calculateChecksum(payload)
	return packet
}

func (p *Packet) Encode() ([]byte, error) {
	totalSize := HeaderSize + len(p.Payload)
	buffer := make([]byte, totalSize)

	// ヘッダーの書き込み（ビッグエンディアン）
	binary.BigEndian.PutUint64(buffer[0:], p.Header.ID)
	binary.BigEndian.PutUint32(buffer[8:], uint32(p.Header.ChunkIndex))
	binary.BigEndian.PutUint32(buffer[12:], p.Header.ChunkSize)
	binary.BigEndian.PutUint32(buffer[16:], p.Header.Checksum)

	// ペイロードのコピー
	copy(buffer[HeaderSize:], p.Payload)

	return buffer, nil
}

func DecodePacket(data []byte) (*Packet, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("packet too short")
	}

	packet := &Packet{
		Header: PacketHeader{
			ID:         binary.BigEndian.Uint64(data[0:]),
			ChunkIndex: int32(binary.BigEndian.Uint32(data[8:])),
			ChunkSize:  binary.BigEndian.Uint32(data[12:]),
			Checksum:   binary.BigEndian.Uint32(data[16:]),
		},
	}

	// ペイロードの抽出
	payloadSize := int(packet.Header.ChunkSize)
	if len(data) < HeaderSize+payloadSize {
		return nil, fmt.Errorf("incomplete packet")
	}

	packet.Payload = make([]byte, payloadSize)
	copy(packet.Payload, data[HeaderSize:HeaderSize+payloadSize])

	// チェックサム検証
	if !packet.ValidateChecksum() {
		return nil, fmt.Errorf("checksum mismatch")
	}

	return packet, nil
}

func (p *Packet) ValidateChecksum() bool {
	return p.Header.Checksum == calculateChecksum(p.Payload)
}

func calculateChecksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
