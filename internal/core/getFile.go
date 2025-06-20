package core

import (
	"QuickPort/tray"
	"fmt"
	"hash/crc32"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

func GetFile(handle *Handle, args *ShellArgs) error {
	if len(args.Arg) < 1 {
		fmt.Println("get peer file\nget [path]")
		return nil
	}

	filePath := args.Head()

	// Step 1: ファイルリクエスト送信
	logrus.Infof("Requesting file: %s", filePath)
	reqData := BaseData{
		Type: FileReqest,
		Data: fileRequestData{
			FilePath: filePath,
		},
	}

	err := Write(handle.Self.Conn, handle.Peer.Addr.StrAddr(), &reqData)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	// Step 2: インデックス情報受信
	logrus.Info("Waiting for file index...")
	indexData, err := receiveFileIndex(handle)
	if err != nil {
		return fmt.Errorf("failed to receive file index: %v", err)
	}

	logrus.Infof("File info - Size: %d bytes, Chunks: %d", indexData.TotalSize, indexData.ChunkCount)

	// Step 3: ファイル受信準備
	outputPath := filepath.Join(tray.UseTray(), filepath.Base(filePath))
	err = os.MkdirAll(filepath.Dir(outputPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Step 4: 受信開始の合図を送信
	startData := BaseData{
		Type: Message,
		Data: map[string]interface{}{"action": "start_transfer"},
	}
	err = Write(handle.Self.Conn, handle.Peer.Addr.StrAddr(), &startData)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	logrus.Info("Sent start transfer signal, receiving file chunks...")

	// Step 5: チャンク受信ループ
	receivedChunks := make(map[uint32]bool)
	missingChunks := make([]uint32, 0)

	// タイムアウト設定
	handle.Self.Conn.SetReadDeadline(time.Now().Add(time.Second * TimeoutSeconds))

	for len(receivedChunks) < int(indexData.ChunkCount) {
		chunk, err := receiveFileChunk(handle.Self.Conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logrus.Warn("Timeout occurred, requesting missing chunks...")
				break
			}
			return fmt.Errorf("failed to receive chunk: %v", err)
		}

		// チェックサム検証
		expectedChecksum := crc32.ChecksumIEEE(chunk.Data)
		if expectedChecksum != chunk.Checksum {
			logrus.Warnf("Checksum mismatch for chunk %d, will request again", chunk.Index)
			continue
		}

		// チャンクをファイルに書き込み
		offset := int64(chunk.Index) * int64(ChunkSize)
		_, err = file.WriteAt(chunk.Data, offset)
		if err != nil {
			return fmt.Errorf("failed to write chunk %d: %v", chunk.Index, err)
		}

		receivedChunks[chunk.Index] = true
		logrus.Debugf("Received chunk %d/%d", len(receivedChunks), indexData.ChunkCount)
	}

	// Step 6: 欠落チャンクの確認と再送要求
	for i := uint32(0); i < indexData.ChunkCount; i++ {
		if !receivedChunks[i] {
			missingChunks = append(missingChunks, i)
		}
	}

	retryCount := 0
	for len(missingChunks) > 0 && retryCount < MaxRetries {
		logrus.Infof("Requesting %d missing chunks (retry %d/%d)", len(missingChunks), retryCount+1, MaxRetries)

		// 欠落チャンクリスト送信
		missingData := BaseData{
			Type: Message,
			Data: MissingPacketData{
				MissingChunks: missingChunks,
			},
		}

		err = Write(handle.Self.Conn, handle.Peer.Addr.StrAddr(), &missingData)
		if err != nil {
			return fmt.Errorf("failed to send request: %v", err)
		}

		// 欠落チャンクの受信
		handle.Self.Conn.SetReadDeadline(time.Now().Add(time.Second * TimeoutSeconds))

		for len(missingChunks) > 0 {
			chunk, err := receiveFileChunk(handle.Self.Conn)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					break
				}
				return fmt.Errorf("failed to receive missing chunk: %v", err)
			}

			// チェックサム検証
			expectedChecksum := crc32.ChecksumIEEE(chunk.Data)
			if expectedChecksum != chunk.Checksum {
				logrus.Warnf("Checksum mismatch for missing chunk %d", chunk.Index)
				continue
			}

			// チャンクをファイルに書き込み
			offset := int64(chunk.Index) * int64(ChunkSize)
			_, err = file.WriteAt(chunk.Data, offset)
			if err != nil {
				return fmt.Errorf("failed to write missing chunk %d: %v", chunk.Index, err)
			}

			// 欠落リストから削除
			for i, missing := range missingChunks {
				if missing == chunk.Index {
					missingChunks = append(missingChunks[:i], missingChunks[i+1:]...)
					break
				}
			}
		}

		retryCount++
	}

	// Step 7: ファイル整合性チェック
	file.Close()
	receivedHash, err := calculateFileHash(outputPath)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %v", err)
	}

	if receivedHash != indexData.FileHash {
		// 終了パケット送信（失敗）
		finishData := BaseData{
			Type: Message,
			Data: FinishPacketData{
				Success: false,
				Message: "File hash mismatch",
			},
		}

		err = Write(handle.Self.Conn, handle.Peer.Addr.StrAddr(), &finishData)
		if err != nil {
			return fmt.Errorf("failed to send request: %v", err)
		}

		return fmt.Errorf("file hash mismatch - expected: %s, got: %s", indexData.FileHash, receivedHash)
	}

	// Step 8: 終了パケット送信（成功）
	finishData := BaseData{
		Type: Message,
		Data: FinishPacketData{
			Success: true,
			Message: "File received successfully",
		},
	}

	err = Write(handle.Self.Conn, handle.Peer.Addr.StrAddr(), &finishData)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	logrus.Infof("File downloaded successfully: %s", outputPath)
	return nil
}
