package peer

import (
	"crypto/sha256"
	"encoding/hex"
)

type ConnectionRequest struct {
	ClientName string `json:"name"`
	ClientPort uint16 `json:"port"`
	TokenHash  string `json:"hash"`
	Version    uint8  `json:"version"`
}

type ConnectionResponse struct {
	Status     uint8  `json:"status"`
	Reason     string `json:"reason,omitempty"`
	SessionID  uint64 `json:"session_id,omitempty"`
	ServerName string `json:"server_name"`
}

type ConnectionConfirm struct {
	SessionID  uint64 `json:"session_id"`
	ClientName string `json:"name"`
}

func calculateTokenHash(token []byte) string {
	hash := sha256.Sum256(token)
	return hex.EncodeToString(hash[:])
}
