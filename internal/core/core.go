package core

import (
	"QuickPort/tray"
	"QuickPort/utils"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net"
	"os"
	"path/filepath"
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

			SendFile(handle, filereq.FilePath)
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

func calculateFileHash(string) (string, error)

func receiveFileIndex(handle *Handle) (*FileIndexData, error) {
	for {
		meta, err := receiveFromPeer(handle.Self, handle.Peer)
		if err != nil {
			return nil, err
		}

		if meta.Type != FileIndex {
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
	//process
	//<-send index
	//->data req
	//<-file data
	//->missing packet list
	//<-send missing packet
	// ~~~~
	//->finish packet
	return nil
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
	outputPath := filepath.Join("./downloads", filepath.Base(filePath))
	err = os.MkdirAll(filepath.Dir(outputPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Step 4: チャンク受信ループ
	receivedChunks := make(map[uint32]bool)
	missingChunks := make([]uint32, 0)

	// 受信開始の合図を送信
	startData := BaseData{
		Type: Message,
		Data: map[string]interface{}{"action": "start_transfer"},
	}
	raw, _ = json.Marshal(startData)
	handle.Self.Conn.WriteToUDP(raw, &net.UDPAddr{
		IP:   handle.Peer.Addr.Ip,
		Port: handle.Peer.Addr.Port,
	})

	logrus.Info("Receiving file chunks...")

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

	// Step 5: 欠落チャンクの確認と再送要求
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

	// Step 6: ファイル整合性チェック
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

	// Step 7: 終了パケット送信（成功）
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

func receiveFromPeer(self *SelfCfg, peer *PeerCfg) (*BaseData, error) {
	buf := make([]byte, 1024)
	for {
		n, peerAddr, err := self.Conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}

		if peerAddr.IP.String() != peer.Addr.Ip.String() || peerAddr.Port != peer.Addr.Port {
			continue
		}

		var meta BaseData
		err = json.Unmarshal(buf[:n], &meta)
		if err != nil {
			return nil, err
		}

		return &meta, nil
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
