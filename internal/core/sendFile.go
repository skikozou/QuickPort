package core

import (
	"QuickPort/tray"
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

func SendFile(handle *Handle, filereq *fileRequestData) error {
	// Step 1: ファイルの存在確認とメタデータ取得
	fullpath := tray.UseTray() + filepath.Clean(filereq.FilePath)
	fileInfo, err := os.Stat(fullpath)
	logrus.Debugf("fileinfo: %v", fileInfo)
	if err != nil {
		logrus.Errorf("File not found: %s", filereq.FilePath)
		return fmt.Errorf("file not found: %v", err)
	}

	if fileInfo.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", filereq.FilePath)
	}

	// Step 2: ファイルハッシュ計算
	fileHash, err := calculateFileHash(fullpath)
	logrus.Debugf("filehash: %s", fileHash)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %v", err)
	}

	// Step 6: ファイルを開く（修正：フルパスを使用）
	file, err := os.Open(fullpath) // ←ここを修正
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	//圧縮処理
	raw, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	compressed, err := Compress(raw, filereq.CompMode)
	if err != nil {
		return err
	}

	compressedSize := int64(len(compressed))
	fileReader := bytes.NewReader(compressed)

	// Step 3: チャンク数計算
	chunkCount := uint32((compressedSize + int64(ChunkSize) - 1) / int64(ChunkSize))
	logrus.Debugf("chunk count: %d", chunkCount)

	// Step 4: ファイルインデックス情報送信
	indexData := BaseData{
		Type: FileIndex,
		Data: FileIndexData{
			FilePath:   filereq.FilePath,
			TotalSize:  compressedSize,
			ChunkCount: chunkCount,
			FileHash:   fileHash,
			ChunkSize:  ChunkSize,
		},
	}

	err = Write(handle.Self.SubConn, handle.Peer.SubAddr.StrAddr(), &indexData)

	if err != nil {
		return fmt.Errorf("failed to send file index: %v", err)
	}

	logrus.Infof("Sent file index - Size: %d bytes, Chunks: %d", fileInfo.Size(), chunkCount)

	// Step 5: 転送開始信号を待機
	logrus.Info("Waiting for transfer start signal...")
	for {
		meta, err := receiveFromPeer(handle.Self, handle.Peer, true)
		if err != nil {
			return fmt.Errorf("failed to receive start signal: %v", err)
		}

		if meta.Type == Message {
			if data, ok := meta.Data.(map[string]interface{}); ok {
				if action, exists := data["action"]; exists && action == "start_transfer" {
					break
				}
			}
		}
	}

	// Step 7: 初回ファイル送信
	logrus.Info("Starting file transmission...")
	err = sendFileChunks(handle, fileReader, chunkCount)
	if err != nil {
		return fmt.Errorf("failed to send file chunks: %v", err)
	}

	// Step 8: 欠落チャンクの再送処理
	retryCount := 0

	for retryCount < MaxRetries {
		// 欠落チャンクリストまたは終了パケットを待機
		logrus.Debug("Waiting for missing chunks request or finish packet...")

		// 分割パケットを受信して結合
		missingChunks, finished, err := receiveMissingChunksList(handle)
		if err != nil {
			return fmt.Errorf("failed to receive missing chunks list: %v", err)
		}

		logrus.Debug(len(missingChunks))

		if finished {
			logrus.Info("File transfer completed successfully")
			return nil
		}

		if len(missingChunks) == 0 {
			logrus.Info("No missing chunks, transfer completed")
			return nil
		}

		logrus.Infof("Resending %d missing chunks (retry %d/%d)",
			len(missingChunks), retryCount+1, MaxRetries)

		err = sendMissingChunks(handle, file, missingChunks)
		if err != nil {
			return fmt.Errorf("failed to resend missing chunks: %v", err)
		}

		retryCount++
	}

	return fmt.Errorf("maximum retries exceeded, file transfer failed")
}

func receiveMissingChunksList(handle *Handle) ([]uint32, bool, error) {
	receivedPackets := make(map[uint32][]uint32)
	var totalPackets uint32

	for {
		meta, err := receiveFromPeer(handle.Self, handle.Peer, true)
		if err != nil {
			return nil, false, fmt.Errorf("failed to receive response: %v", err)
		}

		if meta.Type == Message {
			// 終了パケットの確認
			if finishData, err := convertMapToFinishPacketData(meta.Data); err == nil {
				if finishData.Success {
					return nil, true, nil
				} else {
					return nil, false, fmt.Errorf("file transfer failed: %v", finishData)
				}
			}
		} else if meta.Type == PacketInfo {
			// 欠落チャンクリストの処理
			if missingData, err := convertMapToMissingPacketData(meta.Data); err == nil {
				// 分割パケットの場合
				if missingData.TotalPackets > 1 {
					receivedPackets[missingData.PacketIndex] = missingData.MissingChunks
					totalPackets = missingData.TotalPackets

					// 全てのパケットが受信されたかチェック
					if len(receivedPackets) == int(totalPackets) {
						// パケットを順番に結合
						allMissingChunks := make([]uint32, 0)
						for i := uint32(0); i < totalPackets; i++ {
							if chunks, exists := receivedPackets[i]; exists {
								allMissingChunks = append(allMissingChunks, chunks...)
							}
						}
						return allMissingChunks, false, nil
					}
					// まだ全てのパケットが揃っていない場合は継続
					continue
				} else {
					// 単一パケットの場合（従来の処理）
					return missingData.MissingChunks, false, nil
				}
			}
		}
	}
}

// sendFileChunks sends all file chunks sequentially
func sendFileChunks(handle *Handle, reader io.Reader, chunkCount uint32) error {
	buffer := make([]byte, ChunkSize)

	for i := uint32(0); i < chunkCount; i++ {
		// チャンクデータ読み込み
		n, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read chunk %d: %v", i, err)
		}

		if n == 0 {
			break // データの終端
		}

		// チャンクデータの実際のサイズに調整
		chunkData := buffer[:n]

		// チャンク送信
		err = sendSingleChunk(handle, i, chunkData)
		if err != nil {
			return fmt.Errorf("failed to send chunk %d: %v", i, err)
		}

		// 進捗表示
		if (i+1)%100 == 0 || (i+1) == chunkCount {
			logrus.Debugf("Sent chunk %d/%d", i+1, chunkCount)
		}
	}

	return nil
}

// sendMissingChunks resends specific missing chunks
func sendMissingChunks(handle *Handle, file *os.File, missingChunks []uint32) error {
	buffer := make([]byte, ChunkSize)

	for _, chunkIndex := range missingChunks {
		// ファイルの該当位置にシーク
		offset := int64(chunkIndex) * int64(ChunkSize)
		_, err := file.Seek(offset, 0)
		if err != nil {
			return fmt.Errorf("failed to seek file for missing chunk %d: %v", chunkIndex, err)
		}

		// チャンクデータ読み込み
		n, err := file.Read(buffer)
		if err != nil && err.Error() != "EOF" {
			return fmt.Errorf("failed to read missing chunk %d: %v", chunkIndex, err)
		}

		// チャンクデータの実際のサイズに調整
		chunkData := buffer[:n]

		// チャンク送信
		err = sendSingleChunk(handle, chunkIndex, chunkData)
		if err != nil {
			return fmt.Errorf("failed to resend chunk %d: %v", chunkIndex, err)
		}

		logrus.Debugf("Resent missing chunk %d", chunkIndex)
	}

	return nil
}

// sendSingleChunk sends a single file chunk using custom protocol
func sendSingleChunk(handle *Handle, index uint32, data []byte) error {
	// チェックサム計算
	checksum := crc32.ChecksumIEEE(data)
	length := uint32(len(data))

	// カスタムプロトコルでパケット構成
	// [Index:4][Length:4][Checksum:4][Data:Length]
	packet := make([]byte, 12+length)

	binary.LittleEndian.PutUint32(packet[0:4], index)
	binary.LittleEndian.PutUint32(packet[4:8], length)
	binary.LittleEndian.PutUint32(packet[8:12], checksum)
	copy(packet[12:], data)

	// UDP送信
	_, err := handle.Self.SubConn.WriteToUDP(packet, &net.UDPAddr{
		IP:   handle.Peer.SubAddr.Ip,
		Port: handle.Peer.SubAddr.Port,
	})

	return err
}
