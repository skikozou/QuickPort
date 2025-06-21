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
	"time"

	"github.com/sirupsen/logrus"
)

//独自パケット、json、shell、コマンドでモード分け
//一括レシーバーを作り、chanに受信したのを入れ
//指定した条件に一致の時chanでこっちに値を流す(jsonにできるか、byteのヘッダーが規定通りか、で判断)

func (h *Handle) NewReceiver(buf chan []byte) {
	for {
		basedata, err := receiveFromPeer(h.Self, h.Peer)
		if err != nil {
			logrus.Debugf("Receiver Error: %s", err)
			continue
		}

		switch basedata.Type {
		case FileReqest:
			filereq, err := ConvertMapToFileReqestMeta(basedata.Data)
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
			filereq, err := ConvertMapToFileReqestMeta(basedata.Data)
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

func (h *Handle) ClaudeReceiver(pause <-chan bool) {
	var isPaused bool = false
	logrus.Info("Receiver started")

	for {
		select {
		case p := <-pause:
			isPaused = p
			logrus.Infof("Receiver pause state changed: %v", isPaused)
		default:
			// より長いタイムアウトを設定（通常の処理用）
			timeout := time.Second * 5
			if isPaused {
				// pause中は短いタイムアウトでFileRequestのみチェック
				timeout = time.Millisecond * 500
			}

			h.Self.Conn.SetReadDeadline(time.Now().Add(timeout))
			basedata, err := receiveFromPeer(h.Self, h.Peer)

			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// タイムアウトは正常、continue
					continue
				}
				logrus.Debugf("Receiver Error: %s", err)
				continue
			}

			logrus.Debugf("Received packet type: %v", basedata.Type)

			switch basedata.Type {
			case FileReqest:
				logrus.Info("Received FileRequest")
				filereq, err := ConvertMapToFileReqestMeta(basedata.Data)
				if err != nil {
					logrus.Errorf("Failed to decode FileRequest: %s", err)
					continue
				}

				logrus.Infof("Processing file request for: %s", filereq.FilePath)
				err = SendFile(h, filereq.FilePath)
				if err != nil {
					logrus.Errorf("Failed to send file: %s", err)
				} else {
					logrus.Info("File sent successfully")
				}

			case Message:
				if !isPaused {
					logrus.Debug("Received Message packet")
					// メッセージ処理（pause中でない場合のみ）
				}

			default:
				if !isPaused {
					logrus.Debugf("Received unhandled packet type: %v", basedata.Type)
				}
			}
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
				filereq, err := ConvertMapToFileReqestMeta(basedata.Data)
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

func ReceiveTray(self *SelfConfig, peer *PeerConfig) (*[]tray.FileMeta, error) {
	logrus.Debug("Waiting for tray data from peer...")

	meta, err := receiveFromPeer(self, peer)
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
