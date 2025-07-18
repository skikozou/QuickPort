package core

import (
	"QuickPort/tray"
	"QuickPort/ui"
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
	var compMode string

	if len(args.Arg) >= 2 {
		compMode = args.Next().Head()
	}

	// Step 1: ファイルリクエスト送信
	logrus.Infof("Requesting file: %s", filePath)
	reqData := BaseData{
		Type: FileReqest,
		Data: fileRequestData{
			FilePath: filePath,
			CompMode: compMode,
		},
	}

	err := Write(handle.Self.Conn, handle.Peer.Addr.StrAddr(), &reqData)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	// Step 2: インデックス情報受信 (SubConnを使用)
	logrus.Info("Waiting for file index...")
	indexData, err := receiveFileIndex(handle)
	if err != nil {
		handle.SendError(&ErrorPacketData{Error: "failed to receive file index", Code: FaildReceive}, true)
		//retry
		return fmt.Errorf("failed to receive file index: %v", err)
	}

	logrus.Infof("File info - Size: %d bytes, Chunks: %d", indexData.TotalSize, indexData.ChunkCount)

	// Step 3: ファイル受信準備
	outputPath := filepath.Join(tray.UseTray(), filepath.Base(filePath))
	err = os.MkdirAll(filepath.Dir(outputPath), 0755)
	if err != nil {
		handle.SendError(&ErrorPacketData{Error: "failed to create output directory", Code: FailedFileOperations}, true)
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		handle.SendError(&ErrorPacketData{Error: "failed to create output file", Code: FailedFileOperations}, true)
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Step 4: 受信開始の合図を送信
	startData := BaseData{
		Type: Message,
		Data: map[string]interface{}{"action": "start_transfer"},
	}
	err = Write(handle.Self.SubConn, handle.Peer.SubAddr.StrAddr(), &startData)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	logrus.Info("Sent start transfer signal, receiving file chunks...")

	// Step 5: チャンク受信ループ
	receivedChunks := make(map[uint32][]byte) // チャンクデータも保存
	missingChunks := make([]uint32, 0)

	//チャンクマップのセットアップ (チャンクからマスの計算とか)
	chunks := ui.MakeChunks(int(indexData.ChunkCount))

	for len(receivedChunks) < int(indexData.ChunkCount) {
		// タイムアウト設定
		handle.Self.SubConn.SetReadDeadline(time.Now().Add(time.Second * ChunkTimeoutSeconds))

		chunk, err := receiveFileChunk(handle.Self.SubConn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logrus.Warn("Timeout occurred, requesting missing chunks...")
				break
			}

			handle.SendError(&ErrorPacketData{Error: "failed to receive chunk", Code: FaildReceive}, true)
			return fmt.Errorf("failed to receive chunk: %v", err)
		}

		// チェックサム検証
		expectedChecksum := crc32.ChecksumIEEE(chunk.Data)
		if expectedChecksum != chunk.Checksum {
			logrus.Warnf("Checksum mismatch for chunk %d, will request again", chunk.Index)
			continue
		}

		// チャンクデータを保存（圧縮されたまま）
		//チャンクマップの更新
		go ui.UpdateState(chunks, int(chunk.Index), true)
		receivedChunks[chunk.Index] = chunk.Data
		logrus.Debugf("Received chunk %d/%d", len(receivedChunks), indexData.ChunkCount)
	}

	// Step 6: 欠落チャンクの確認と再送要求
	for i := uint32(0); i < indexData.ChunkCount; i++ {
		if _, exists := receivedChunks[i]; !exists {
			missingChunks = append(missingChunks, i)
		}
	}

	retryCount := 0
	for len(missingChunks) > 0 && retryCount < MaxRetries {
		logrus.Infof("Requesting %d missing chunks (retry %d/%d)", len(missingChunks), retryCount+1, MaxRetries)

		// 欠落チャンクリスト送信
		err = sendMissingChunksList(handle, missingChunks)
		if err != nil {
			return fmt.Errorf("failed to send missing chunks list: %v", err)
		}

		for len(missingChunks) > 0 {
			// 欠落チャンクの受信
			handle.Self.SubConn.SetReadDeadline(time.Now().Add(time.Second * MissingChunkTimeoutSeconds))

			chunk, err := receiveFileChunk(handle.Self.SubConn)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					break
				}

				handle.SendError(&ErrorPacketData{Error: "failed to receive missing chunk", Code: FaildReceive}, true)

				logrus.Debug(fmt.Sprintf("failed to receive missing chunk: %v", err))
				continue
			}

			// チェックサム検証
			expectedChecksum := crc32.ChecksumIEEE(chunk.Data)
			if expectedChecksum != chunk.Checksum {
				logrus.Warnf("Checksum mismatch for missing chunk %d", chunk.Index)
				continue
			}

			// チャンクデータを保存
			receivedChunks[chunk.Index] = chunk.Data

			//チャンクマップの更新
			// 欠落リストから削除
			go ui.UpdateState(chunks, int(chunk.Index), true)
			for i, missing := range missingChunks {
				if missing == chunk.Index {
					missingChunks = append(missingChunks[:i], missingChunks[i+1:]...)
					break
				}
			}
		}

		retryCount++
	}

	// Step 7: 全チャンクを結合して展開
	ui.ClearState(chunks)
	logrus.Info("Reconstructing file from chunks...")

	// 圧縮されたデータを結合
	compressedData := make([]byte, 0, indexData.TotalSize)
	for i := uint32(0); i < indexData.ChunkCount; i++ {
		if chunkData, exists := receivedChunks[i]; exists {
			compressedData = append(compressedData, chunkData...)
		} else {
			handle.SendError(&ErrorPacketData{Error: "failed to receive chunk", Code: FaildReceive}, true)
			//retry
			return fmt.Errorf("missing chunk %d after retry", i)
		}
	}

	// 圧縮されたデータを展開
	decompressedData, err := Decompress(compressedData, compMode)
	if err != nil {
		handle.SendError(&ErrorPacketData{Error: "failed to decompress", Code: FailedDeCompress}, true)
		return fmt.Errorf("failed to decompress data: %v", err)
	}

	// 展開されたデータをファイルに書き込み
	_, err = file.Write(decompressedData)
	if err != nil {
		handle.SendError(&ErrorPacketData{Error: "failed to write decompressed data", Code: FailedFileOperations}, true)
		return fmt.Errorf("failed to write decompressed data: %v", err)
	}

	// Step 8: ファイル整合性チェック
	file.Close()
	receivedHash, err := calculateFileHash(outputPath)
	if err != nil {
		handle.SendError(&ErrorPacketData{Error: "failed to calculate file hash", Code: FailedCalcFileHash}, true)
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

		os.Remove(outputPath)

		err = Write(handle.Self.SubConn, handle.Peer.SubAddr.StrAddr(), &finishData)
		if err != nil {
			return fmt.Errorf("failed to send request: %v", err)
		}

		return fmt.Errorf("file hash mismatch - expected: %s, got: %s", indexData.FileHash, receivedHash)
	}

	// Step 9: 終了パケット送信（成功）
	finishData := BaseData{
		Type: Message,
		Data: FinishPacketData{
			Success: true,
			Message: "File received successfully",
		},
	}

	err = Write(handle.Self.SubConn, handle.Peer.SubAddr.StrAddr(), &finishData)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	logrus.Infof("File downloaded successfully: %s", outputPath)
	return nil
}

func sendMissingChunksList(handle *Handle, missingChunks []uint32) error {
	const maxChunksPerPacket = 135 // 1つのパケットで送信可能な最大チャンク数

	// 分割パケット数を計算
	totalPackets := (len(missingChunks) + maxChunksPerPacket - 1) / maxChunksPerPacket

	for i := 0; i < totalPackets; i++ {
		start := i * maxChunksPerPacket
		end := start + maxChunksPerPacket
		if end > len(missingChunks) {
			end = len(missingChunks)
		}

		// 分割されたチャンクリスト
		chunkSlice := missingChunks[start:end]

		// 分割パケットデータ
		packetData := MissingPacketData{
			MissingChunks: chunkSlice,
			PacketIndex:   uint32(i),
			TotalPackets:  uint32(totalPackets),
		}

		missingData := BaseData{
			Type: PacketInfo,
			Data: packetData,
		}

		err := Write(handle.Self.SubConn, handle.Peer.SubAddr.StrAddr(), &missingData)
		if err != nil {
			return fmt.Errorf("failed to send missing chunk packet %d/%d: %v", i+1, totalPackets, err)
		}

		logrus.Debugf("Sent missing chunk packet %d/%d with %d chunks", i+1, totalPackets, len(chunkSlice))
	}

	return nil
}
