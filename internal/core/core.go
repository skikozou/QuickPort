package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pion/stun"
	"github.com/sirupsen/logrus"
)

func Reciever(handle *Handle) {
	for {
		basedata, err := receiveFromPeer(handle.Self, handle.Peer)
		if err != nil {
			logrus.Debugf("Reciever Error: %s", err)
			continue
		}

		switch basedata.Type {
		case FileReqest:
			filereq, err := ConvertMapToFileReqMeta(basedata.Data)
			if err != nil {
				logrus.Errorf("Decode Error: %s", err)
				continue
			}

			err = SendFile(handle, filereq.FilePath)
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
	for {
		meta, err := receiveFromPeer(handle.Self, handle.Peer)
		if err != nil {
			return nil, err
		}
		fmt.Println("geasg") //こいつがでない！！！！！！！！！！！！！！！！！！！！！！！！！

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

func SendFile(handle *Handle, path string) error {
	// Step 1: ファイルの存在確認とメタデータ取得
	fullpath := tray.UseTray() + filepath.Clean(path)
	fileInfo, err := os.Stat(fullpath)
	logrus.Debugf("fileinfo: %v", fileInfo)
	if err != nil {
		logrus.Errorf("File not found: %s", path)
		return fmt.Errorf("file not found: %v", err)
	}

	if fileInfo.IsDir() {
		return fmt.Errorf("path is a directory, not a file: %s", path)
	}

	// Step 2: ファイルハッシュ計算
	fileHash, err := calculateFileHash(fullpath)
	logrus.Debugf("filehash: %s", fileHash)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %v", err)
	}

	// Step 3: チャンク数計算
	chunkCount := uint32((fileInfo.Size() + int64(ChunkSize) - 1) / int64(ChunkSize))
	logrus.Debugf("chunk count: %d", chunkCount)

	// Step 4: ファイルインデックス情報送信
	indexData := BaseData{
		Type: FileIndex,
		Data: FileIndexData{
			FilePath:   path,
			TotalSize:  fileInfo.Size(),
			ChunkCount: chunkCount,
			FileHash:   fileHash,
			ChunkSize:  ChunkSize,
		},
	}

	err = Write(handle.Self.Conn, handle.Peer.Addr.StrAddr(), &indexData)

	if err != nil {
		return fmt.Errorf("failed to send file index: %v", err)
	}

	logrus.Infof("Sent file index - Size: %d bytes, Chunks: %d", fileInfo.Size(), chunkCount)

	// Step 5: 転送開始信号を待機
	logrus.Info("Waiting for transfer start signal...")
	for {
		meta, err := receiveFromPeer(handle.Self, handle.Peer)
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

	// Step 6: ファイルを開く（修正：フルパスを使用）
	file, err := os.Open(fullpath) // ←ここを修正
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Step 7: 初回ファイル送信
	logrus.Info("Starting file transmission...")
	err = sendFileChunks(handle, file, chunkCount)
	if err != nil {
		return fmt.Errorf("failed to send file chunks: %v", err)
	}

	// Step 8: 欠落チャンクの再送処理
	retryCount := 0
	for retryCount < MaxRetries {
		// 欠落チャンクリストまたは終了パケットを待機
		logrus.Debug("Waiting for missing chunks request or finish packet...")

		meta, err := receiveFromPeer(handle.Self, handle.Peer)
		if err != nil {
			return fmt.Errorf("failed to receive response: %v", err)
		}

		if meta.Type == Message {
			// 終了パケットの確認
			if finishData, err := convertMapToFinishPacketData(meta.Data); err == nil {
				if finishData.Success {
					logrus.Info("File transfer completed successfully")
					return nil
				} else {
					return fmt.Errorf("file transfer failed: %s", finishData.Message)
				}
			}

			// 欠落チャンクリストの処理
			if missingData, err := convertMapToMissingPacketData(meta.Data); err == nil {
				if len(missingData.MissingChunks) == 0 {
					logrus.Info("No missing chunks, transfer completed")
					return nil
				}

				logrus.Infof("Resending %d missing chunks (retry %d/%d)",
					len(missingData.MissingChunks), retryCount+1, MaxRetries)

				err = sendMissingChunks(handle, file, missingData.MissingChunks)
				if err != nil {
					return fmt.Errorf("failed to resend missing chunks: %v", err)
				}

				retryCount++
			}
		}
	}

	return fmt.Errorf("maximum retries exceeded, file transfer failed")
}

// sendFileChunks sends all file chunks sequentially
func sendFileChunks(handle *Handle, file *os.File, chunkCount uint32) error {
	buffer := make([]byte, ChunkSize)

	for i := uint32(0); i < chunkCount; i++ {
		// ファイルの該当位置にシーク
		offset := int64(i) * int64(ChunkSize)
		_, err := file.Seek(offset, 0)
		if err != nil {
			return fmt.Errorf("failed to seek file at offset %d: %v", offset, err)
		}

		// チャンクデータ読み込み
		n, err := file.Read(buffer)
		if err != nil && err.Error() != "EOF" {
			return fmt.Errorf("failed to read chunk %d: %v", i, err)
		}

		// チャンクデータの実際のサイズに調整
		chunkData := buffer[:n]

		// チャンク送信
		err = sendSingleChunk(handle, i, chunkData)
		if err != nil {
			return fmt.Errorf("failed to send chunk %d: %v", i, err)
		}

		// 進捗表示（デバッグ用）
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
	_, err := handle.Self.Conn.WriteToUDP(packet, &net.UDPAddr{
		IP:   handle.Peer.Addr.Ip,
		Port: handle.Peer.Addr.Port,
	})

	return err
}

// Helper functions for data conversion
func convertMapToMissingPacketData(input interface{}) (*MissingPacketData, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data MissingPacketData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func convertMapToFinishPacketData(input interface{}) (*FinishPacketData, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data FinishPacketData
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

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
		Data: FileReqData{
			FilePath: filePath,
		},
	}

	raw, err := json.Marshal(reqData)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %v", err)
	}

	_, err = handle.Self.Conn.WriteToUDP(raw, &net.UDPAddr{
		IP:   handle.Peer.Addr.Ip,
		Port: handle.Peer.Addr.Port,
	})
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
	raw, err = json.Marshal(startData)
	if err != nil {
		return fmt.Errorf("failed to marshal start signal: %v", err)
	}

	_, err = handle.Self.Conn.WriteToUDP(raw, &net.UDPAddr{
		IP:   handle.Peer.Addr.Ip,
		Port: handle.Peer.Addr.Port,
	})
	if err != nil {
		return fmt.Errorf("failed to send start signal: %v", err)
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

		raw, err := json.Marshal(missingData)
		if err != nil {
			return fmt.Errorf("failed to marshal missing chunks: %v", err)
		}

		_, err = handle.Self.Conn.WriteToUDP(raw, &net.UDPAddr{
			IP:   handle.Peer.Addr.Ip,
			Port: handle.Peer.Addr.Port,
		})
		if err != nil {
			return fmt.Errorf("failed to send missing chunks request: %v", err)
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
		raw, _ = json.Marshal(finishData)
		handle.Self.Conn.WriteToUDP(raw, &net.UDPAddr{
			IP:   handle.Peer.Addr.Ip,
			Port: handle.Peer.Addr.Port,
		})

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

	raw, err = json.Marshal(finishData)
	if err == nil {
		handle.Self.Conn.WriteToUDP(raw, &net.UDPAddr{
			IP:   handle.Peer.Addr.Ip,
			Port: handle.Peer.Addr.Port,
		})
	}

	logrus.Infof("File downloaded successfully: %s", outputPath)
	return nil
}

func GetLocalAddr() (*Address, error) {
	// 利用可能なネットワークインターフェースを取得
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// ループバックや無効なインターフェースをスキップ
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// IPv4のみを対象とし、プライベートIPアドレスを優先
			if ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
				// プライベートIPアドレスかチェック
				if isPrivateIP(ipNet.IP) {
					return &Address{
						Ip:   ipNet.IP,
						Port: utils.GetPort(),
					}, nil
				}
			}
		}
	}

	// プライベートIPが見つからない場合は、外部接続で取得
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return &Address{
		Ip:   conn.LocalAddr().(*net.UDPAddr).IP,
		Port: utils.GetPort(),
	}, nil
}

func GetLocalIPAlternative() (net.IP, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				// IPv4アドレスで、プライベートアドレス範囲をチェック
				if isPrivateIP(ipnet.IP) {
					return ipnet.IP, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no suitable local IP address found")
}
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"192.168.0.0/16", // 192.168.0.0 - 192.168.255.255
	}

	for _, cidr := range privateRanges {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

func PortSetUp() (*SelfCfg, error) {
	self := SelfCfg{}

	fmt.Printf("Enter your name: ")
	tty, err := utils.UseTty()
	if err != nil {
		return nil, err
	}

	self.Name, err = tty.ReadString()
	if err != nil {
		return nil, err
	}

	self.LocalAddr, err = GetLocalAddr()
	if err != nil {
		// 代替方法を試す
		logrus.Warn("Primary IP detection failed, trying alternative method")
		ip, altErr := GetLocalIPAlternative()
		if altErr != nil {
			return nil, fmt.Errorf("failed to get local IP: %v (alternative: %v)", err, altErr)
		}
		self.LocalAddr = &Address{
			Ip:   ip,
			Port: utils.GetPort(),
		}
	}

	return &self, err
}

// TrayReceive 関数名を修正（元の TrayRecieve は typo）
func TrayReceive(self *SelfCfg, peer *PeerCfg) (*[]tray.FileMeta, error) {
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

// Sync 関数を改善
func Sync(self *SelfCfg, peer *PeerCfg) (*PeerCfg, error) {
	port := utils.GetPort()
	conn, err := ListenUDP(port)
	if err != nil {
		return nil, err
	}

	self.Conn = conn
	logrus.Infof("Listening on %s:%d", self.LocalAddr.Ip.String(), port)

	// 認証リクエスト送信
	addr := fmt.Sprintf("%s:%d", peer.Addr.Ip.String(), peer.Addr.Port)
	logrus.Debug("Sending auth request to:", addr)

	err = Write(conn, addr, &BaseData{
		Type: Auth,
		Data: tray.AuthMeta{Name: self.Name, Flag: tray.AccessReq},
	})
	if err != nil {
		logrus.Error("Failed to send auth request:", err)
		return nil, err
	}

	// 認証レスポンス受信
	logrus.Debug("Waiting for auth response...")
	meta, err := receiveFromPeer(self, peer)
	if err != nil {
		logrus.Error("Failed to receive auth response:", err)
		return nil, err
	}

	if meta.Type != Auth {
		logrus.Error("Invalid response type")
		return nil, fmt.Errorf("invalid response type: %d", meta.Type)
	}

	authmeta, err := ConvertMapToAuthMeta(meta.Data)
	if err != nil {
		logrus.Error("Failed to parse auth meta:", err)
		return nil, err
	}

	switch authmeta.Flag {
	case tray.AccessReq:
		return nil, fmt.Errorf("invalid packet - received request instead of response")
	case tray.Allow:
		logrus.Info("Connection accepted!")
	case tray.Deny:
		logrus.Info("Connection denied by peer")
		return nil, nil
	}

	return peer, nil
}

// SyncListener 関数を改善
func SyncListener(self *SelfCfg) (*PeerCfg, error) {
	port := utils.GetPort()
	conn, err := ListenUDP(port)
	if err != nil {
		return nil, err
	}

	self.Conn = conn
	self.LocalAddr.Port = port

	logrus.Infof("Listening on %s:%d", self.LocalAddr.Ip.String(), port)

	buf := make([]byte, 1024)
waitPeer:
	for {
		n, peerAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			logrus.Error("UDP read error:", err)
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Error("JSON unmarshal error:", err)
			continue
		}

		if meta.Type != Auth {
			logrus.Debug("Ignoring non-auth packet")
			continue
		}

		authmeta, err := ConvertMapToAuthMeta(meta.Data)
		if err != nil {
			logrus.Error("Failed to parse auth meta:", err)
			continue
		}

		if authmeta.Flag != tray.AccessReq {
			logrus.Debug("Ignoring non-request auth packet")
			continue
		}

		tty, err := utils.UseTty()
		if err != nil {
			return nil, err
		}

		for {
			fmt.Printf("%s (%s:%d) is requesting to connect. Accept? (y/n)\n>",
				authmeta.Name, peerAddr.IP.String(), peerAddr.Port)
			answer, err := tty.ReadString()
			if err != nil {
				return nil, err
			}

			peer := &PeerCfg{
				Name: authmeta.Name,
				Addr: &Address{
					Ip:   peerAddr.IP,
					Port: peerAddr.Port,
				},
			}

			switch answer {
			case "y":
				// 承認レスポンス送信
				err = Write(conn, fmt.Sprintf("%s:%d", peerAddr.IP.String(), peerAddr.Port),
					&BaseData{Type: Auth, Data: tray.AuthMeta{Name: self.Name, Flag: tray.Allow}})
				if err != nil {
					logrus.Error("Failed to send allow response:", err)
					return nil, err
				}

				logrus.Info("Connection accepted!")
				return peer, nil

			case "n":
				// 拒否レスポンス送信
				err = Write(conn, fmt.Sprintf("%s:%d", peerAddr.IP.String(), peerAddr.Port),
					&BaseData{Type: Auth, Data: tray.AuthMeta{Name: self.Name, Flag: tray.Deny}})
				if err != nil {
					logrus.Error("Failed to send deny response:", err)
				}

				logrus.Info("Connection denied, waiting for other peer...")
				continue waitPeer

			default:
				fmt.Println("Please enter 'y' or 'n'")
			}
		}
	}
}

func TraySync(self *SelfCfg, peer *PeerCfg, defaultTray string) error {
	items, err := tray.GetTrayItems(defaultTray)
	if err != nil {
		return err
	}

	err = Write(self.Conn, fmt.Sprintf("%s:%d", peer.Addr.Ip.String(), peer.Addr.Port), &BaseData{
		Type: SyncTray,
		Data: items,
	})

	if err != nil {
		return err
	}

	return nil
}

func receiveFromPeer(self *SelfCfg, peer *PeerCfg) (BaseData, error) {
	buf := make([]byte, 1024)
	for {
		n, peerAddr, err := self.Conn.ReadFromUDP(buf)
		if err != nil {
			logrus.Debug(err)
			continue
		}

		if peerAddr.IP.String() != peer.Addr.Ip.String() || peerAddr.Port != peer.Addr.Port {
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			return BaseData{}, err
		}

		logrus.Debug("<<< receiveFromPeer: about to RETURN")
		fmt.Println("ちんちん")
		return meta, nil
	}
}

// UDPポートをバインドして、リッスン状態にする
func ListenUDP(port int) (*net.UDPConn, error) {
	addr := net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// STUNを使って外部アドレスを取得
func GetExternalAddress() (*stun.XORMappedAddress, error) {
	conn, err := net.Dial("udp", "stun.l.google.com:19302")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	c, err := stun.NewClient(conn)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	var resultAddr *stun.XORMappedAddress

	err = c.Do(stun.MustBuild(stun.TransactionID, stun.BindingRequest), func(e stun.Event) {
		if e.Error != nil {
			err = e.Error
			return
		}
		addr := &stun.XORMappedAddress{}
		if parseErr := addr.GetFrom(e.Message); parseErr != nil {
			err = parseErr
			return
		}
		resultAddr = addr
	})
	if err != nil {
		return nil, err
	}

	return resultAddr, nil
}

// pingメッセージを送信
func SendPing(conn *net.UDPConn, targetAddr string) error {
	raddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP([]byte("ping"), raddr)
	return err
}

// data share
func Write(conn *net.UDPConn, targetAddr string, data *BaseData) error {
	raddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = conn.WriteToUDP(raw, raddr)
	return err
}

func ReceiveSync(conn *net.UDPConn) (*BaseData, error) {
	buf := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Fatal(err)
			return nil, err
		}

		if meta.Type != SyncTray {
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
		logrus.Info("Received from %s", addr.String())

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			logrus.Fatal(err)
			return
		}

		ConvertMapToFileMeta(meta.Data)
	}
}

func ConvertMapToFileReqMeta(input any) (*FileReqData, error) {
	// input が map[string]any だと仮定
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta FileReqData
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func ConvertMapToFileMeta(input any) (*[]tray.FileMeta, error) {
	// input が map[string]any だと仮定
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta []tray.FileMeta
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}
func ConvertMapToAuthMeta(input any) (*tray.AuthMeta, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var meta tray.AuthMeta
	err = json.Unmarshal(bytes, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}
func (a *Address) StrAddr() string {
	return a.Ip.String() + ":" + strconv.Itoa(a.Port)
}
