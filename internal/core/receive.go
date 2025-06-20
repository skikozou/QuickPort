package core

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
)

//独自パケット、json、shell、コマンドでモード分け

func (h *Handle) NewReceiver(buf chan []byte) {
	for {
		basedata, err := receiveFromPeer(h.Self, h.Peer)
		if err != nil {
			logrus.Debugf("Receiver Error: %s", err)
			continue
		}

		switch basedata.Type {
		case FileReqest:
			filereq, err := ConvertMapToFileReqMeta(basedata.Data)
			if err != nil {
				logrus.Errorf("Decode Error: %s", err)
				continue
			}

			err = SendFile(h, filereq.FilePath)
			if err != nil {
				logrus.Error(err)
			}
		}
	}
}

// 非推奨
func (h *Handle) ReceiverOld() {
	for {
		basedata, err := receiveFromPeer(h.Self, h.Peer)
		if err != nil {
			// net.Error型の場合のタイムアウトチェックを追加
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // タイムアウトは正常なので continue
			}
			logrus.Debugf("Receiver Error: %s", err)
			continue
		}

		logrus.Info("ぱけっつきた")

		switch basedata.Type {
		case FileReqest:
			filereq, err := ConvertMapToFileReqMeta(basedata.Data)
			if err != nil {
				logrus.Errorf("Decode Error: %s", err)
				continue
			}

			err = SendFile(h, filereq.FilePath)
			if err != nil {
				logrus.Error(err)
			}
			//process
			//<-send index
			//->data req
			//<-file data
			//->missing packet list
			//<-send missing packet
			// ~~~~
			//->finish packet
		case Message:

		}
	}
}

func (h *Handle) Receiver(pause <-chan bool) {
	var pauseflag bool = false

	for {
		select {
		case p := <-pause:
			pauseflag = p
		default:
			if pauseflag {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			h.Self.Conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			basedata, err := receiveFromPeer(h.Self, h.Peer)
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
				filereq, err := ConvertMapToFileReqMeta(basedata.Data)
				if err != nil {
					logrus.Errorf("Decode Error: %s", err)
					continue
				}

				err = SendFile(h, filereq.FilePath)
				if err != nil {
					logrus.Error(err)
				}
			case Message:
				// 他のメッセージ処理
			}
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
		meta, err := receiveFromPeer(handle.Self, handle.Peer)
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
