package peer

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type TokenData struct {
	Version   uint8  `json:"v"`
	Name      string `json:"n"`
	IP        net.IP `json:"i"`
	Port      uint16 `json:"p"`
	Timestamp int64  `json:"t"`
	Salt      []byte `json:"s"`
}

func GenerateToken(name string, ip net.IP, port uint16) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	data := TokenData{
		Version:   1,
		Name:      name,
		IP:        ip,
		Port:      port,
		Timestamp: time.Now().Unix(),
		Salt:      salt,
	}

	// JSONエンコード
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}

	// zlib圧縮
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write(jsonData); err != nil {
		return "", fmt.Errorf("failed to compress token: %w", err)
	}
	w.Close()

	// enc52エンコード
	encoded := Encode(compressed.Bytes())
	return encoded, nil
}

func ParseToken(token string) (*TokenData, error) {
	// enc52デコード
	raw, err := Decode(token)
	if err != nil {
		return nil, fmt.Errorf("enc52 decode failed: %w", err)
	}

	// zlib解凍
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("zlib decompress failed: %w", err)
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(zr); err != nil {
		return nil, fmt.Errorf("zlib read failed: %w", err)
	}

	// JSONデコード
	var td TokenData
	if err := json.Unmarshal(buf.Bytes(), &td); err != nil {
		return nil, fmt.Errorf("json decode failed: %w", err)
	}

	// タイムスタンプ検証（5分以内）
	if time.Now().Unix()-td.Timestamp > 300 {
		return nil, fmt.Errorf("token expired")
	}

	return &td, nil
}
