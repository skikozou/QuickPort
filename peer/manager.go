package peer

import (
	"fmt"
	"net"
	"sync"

	"QuickPort/network"
)

type PeerManager struct {
	conn      *network.Connection
	localName string
	peerName  string
	peerAddr  *net.UDPAddr
	sessionID uint64
	mu        sync.RWMutex
	status    SessionStatus
}

type SessionStatus int

const (
	StatusDisconnected SessionStatus = iota
	StatusConnecting
	StatusConnected
	StatusReady
)

func NewPeerManager(conn *network.Connection, localName string) *PeerManager {
	return &PeerManager{
		conn:      conn,
		localName: localName,
		status:    StatusDisconnected,
	}
}

func (pm *PeerManager) Connect(token string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.status != StatusDisconnected {
		return fmt.Errorf("already connected or connecting")
	}

	// トークンのパース
	tokenData, err := ParseToken(token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// 接続先アドレスの設定
	pm.peerAddr = &net.UDPAddr{
		IP:   tokenData.IP,
		Port: int(tokenData.Port),
	}
	pm.peerName = tokenData.Name
	pm.status = StatusConnecting

	// 接続要求の送信
	return pm.sendConnectionRequest(tokenData)
}

func (pm *PeerManager) sendConnectionRequest(tokenData *TokenData) error {
	// ... 接続要求パケットの作成と送信の実装
	return nil
}

func (pm *PeerManager) HandlePacket(packet []byte) error {
	// ... パケット処理の実装
	return nil
}

func (pm *PeerManager) Status() SessionStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.status
}

func (pm *PeerManager) LocalName() string {
	return pm.localName
}

func (pm *PeerManager) ConnectAndWait(token string) error {
	pm.mu.Lock()
	if pm.status != StatusDisconnected {
		pm.mu.Unlock()
		return fmt.Errorf("already connected or connecting")
	}
	pm.mu.Unlock()

	// トークンのパース
	tokenData, err := ParseToken(token)
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	pm.mu.Lock()
	pm.peerAddr = &net.UDPAddr{
		IP:   tokenData.IP,
		Port: int(tokenData.Port),
	}
	pm.peerName = tokenData.Name
	pm.status = StatusConnecting
	pm.conn.SetRemoteAddr(pm.peerAddr)
	pm.mu.Unlock()

	// 接続要求の送信
	req := ConnectionRequest{
		ClientName: pm.localName,
		ClientPort: uint16(pm.conn.LocalPort()),
		TokenHash:  calculateTokenHash([]byte(token)),
		Version:    tokenData.Version,
	}
	packetMgr := network.NewPacketManager(pm.conn)
	if err := packetMgr.SendControlPacket(network.PacketSubTypeConnectionRequest, req); err != nil {
		return fmt.Errorf("failed to send connection request: %w", err)
	}

	// 応答待ち
	retries := 3
	for retries > 0 {
		packet, err := packetMgr.ReceivePacket()
		if err != nil {
			retries--
			if retries == 0 {
				return fmt.Errorf("failed to receive response: %w", err)
			}
			continue
		}
		if packet.Header.ChunkIndex != -1 {
			continue // 制御パケットのみ処理
		}
		var resp ConnectionResponse
		if err := packetMgr.DecodePayload(packet, &resp); err != nil {
			continue
		}
		if resp.Status != 0 {
			return fmt.Errorf("connection rejected: %s", resp.Reason)
		}
		pm.sessionID = resp.SessionID
		pm.status = StatusReady
		break
	}
	return nil
}
