package network

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

type PacketManager struct {
	nextPacketID uint64
	conn         *Connection
}

func NewPacketManager(conn *Connection) *PacketManager {
	return &PacketManager{
		conn: conn,
	}
}

func (pm *PacketManager) getNextPacketID() uint64 {
	return atomic.AddUint64(&pm.nextPacketID, 1)
}

func (pm *PacketManager) SendControlPacket(subType uint8, payload interface{}) error {
	// ペイロードのJSONエンコード
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 制御パケットの作成
	packet := NewPacket(pm.getNextPacketID(), -1, data)

	// パケットのエンコード
	encodedPacket, err := packet.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode packet: %w", err)
	}

	// パケットの送信
	return pm.conn.Send(encodedPacket)
}

func (pm *PacketManager) ReceivePacket() (*Packet, error) {
	// パケットの受信
	data, _, err := pm.conn.Receive(pm.conn.config.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to receive data: %w", err)
	}

	// パケットのデコード
	packet, err := DecodePacket(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode packet: %w", err)
	}

	return packet, nil
}

func (pm *PacketManager) DecodePayload(packet *Packet, v interface{}) error {
	return json.Unmarshal(packet.Payload, v)
}
