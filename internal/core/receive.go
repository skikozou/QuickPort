package core

import (
	"QuickPort/tray"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
)

func (h *Handle) Receiver() {
	for {
		basedata, err := receiveFromPeer(h.Self, h.Peer, false)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			logrus.Debugf("Receiver Error: %s", err)
			continue
		}

		switch basedata.Type {
		case FileReqest:
			// FileRequestが来たらpauseを無視してSendFileを実行
			filereq, err := ConvertMapToFileReqestMeta(basedata.Data)
			if err != nil {
				logrus.Errorf("Decode Error: %s", err)
				continue
			}

			err = SendFile(h, filereq)
			if err != nil {
				logrus.Error(err)
			}

			//rewrite Prefix
			fmt.Printf("> ")
		case Message:
			// 他のメッセージ処理
		}
	}
}

func calculateFileHash(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	h := fnv.New32a()
	h.Write([]byte(raw))
	return strconv.FormatUint(uint64(h.Sum32()), 10), nil
}

func receiveFileIndex(handle *Handle) (*FileIndexData, error) {
	//logrus.Debug(receiveFromPeer(handle.Self, handle.Peer))
	for {
		meta, err := receiveFromPeer(handle.Self, handle.Peer, true)
		if err != nil {
			return nil, err
		}

		if meta.Type != FileIndex {
			logrus.Debugf("Ignoring packet type: %d, waiting for FileIndex", meta.Type) // ←ログレベルを修正
			continue
		}

		bytes, err := json.Marshal(meta.Data)
		if err != nil {
			return nil, err
		}

		var indexData FileIndexData
		err = json.Unmarshal(bytes, &indexData)
		if err != nil {
			return nil, err
		}

		return &indexData, nil
	}
}

// receiveFileChunk receives file chunk using custom protocol
func receiveFileChunk(conn *net.UDPConn) (*FileChunk, error) {
	buf := make([]byte, ChunkSize+16) // チャンクサイズ + ヘッダー

	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}

	if n < 12 { // 最小ヘッダーサイズ
		return nil, fmt.Errorf("packet too small: %d bytes", n)
	}

	// カスタムプロトコルのパース
	// [Index:4][Length:4][Checksum:4][Data:Length]
	index := binary.LittleEndian.Uint32(buf[0:4])
	length := binary.LittleEndian.Uint32(buf[4:8])
	checksum := binary.LittleEndian.Uint32(buf[8:12])

	if n < int(12+length) {
		return nil, fmt.Errorf("incomplete chunk: expected %d bytes, got %d", 12+length, n)
	}

	data := buf[12 : 12+length]

	return &FileChunk{
		Index:    index,
		Length:   length,
		Checksum: checksum,
		Data:     data,
	}, nil
}

func ReceiveTray(self *SelfConfig, peer *PeerConfig) (*[]tray.FileMeta, error) {
	logrus.Debug("Waiting for tray data from peer...")

	meta, err := receiveFromPeer(self, peer, false)
	if err != nil {
		logrus.Error("Failed to receive from peer:", err)
		return nil, err
	}

	if meta.Type != SyncTray {
		logrus.Error("Invalid packet type, expected SyncTray")
		return nil, fmt.Errorf("invalid packet type: %d", meta.Type)
	}

	logrus.Debug("Received tray sync packet")
	return ConvertMapToFileMeta(meta.Data)
}

func ReceiveSync(conn *net.UDPConn) (*BaseData, error) {
	buf := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			logrus.Debug(err)
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Fatal(err)
			return nil, err
		}

		if meta.Type != SyncTray {
			logrus.Debug("is not Tray Sync")
			continue
		}

		//Read(&meta)
		//ConvertMapToFileMeta()

		return &meta, nil
	}
}

// 受信処理
func ReceiveLoop(conn *net.UDPConn) {
	buf := make([]byte, 1024*1024)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			logrus.Errorln("Read error:", err)
			continue
		}
		logrus.Infof("Received from %s", addr.String())

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Fatal(err)
			return
		}

		ConvertMapToFileMeta(meta.Data)
	}
}
