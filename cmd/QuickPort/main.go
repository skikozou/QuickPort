package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"QuickPort/network"
	"QuickPort/peer"

	"github.com/sirupsen/logrus"
)

func main() {
	// コマンドライン引数の解析
	port := flag.Int("port", 0, "Local port number (0 for random)")
	token := flag.String("token", "", "Connection token (empty for host mode)")
	flag.Parse()

	// ロガーの設定
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// 接続の設定と確立
	config := network.ConnectionConfig{
		LocalPort:     *port,
		BufferSize:    65507,
		Timeout:       5 * time.Second,
		MaxRetries:    3,
		RetryInterval: 3 * time.Second,
	}

	conn, err := network.NewConnection(config)
	if err != nil {
		logrus.Fatalf("Failed to create connection: %v", err)
	}
	defer conn.Close()

	// ピアマネージャーの作成
	pm := peer.NewPeerManager(conn, os.Getenv("USER"))

	// モードに応じた処理
	if *token == "" {
		// ホストモード
		token, err := peer.GenerateToken(pm.LocalName(), nil, uint16(*port))
		if err != nil {
			logrus.Fatalf("Failed to generate token: %v", err)
		}
		fmt.Printf("Your connection token: %s\n", token)

		// 接続待ち受け
		handleHostMode(pm)
	} else {
		// クライアントモード
		if err := pm.Connect(*token); err != nil {
			logrus.Fatalf("Failed to connect: %v", err)
		}

		// 接続確立待ち
		handleClientMode(pm)
	}
}

func handleHostMode(pm *peer.PeerManager) {
	fmt.Println("Waiting for peer connection...")

	for {
		pm.mu.RLock()
		status := pm.status
		pm.mu.RUnlock()
		if status == peer.StatusReady {
			fmt.Println("Peer connected!")
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	// ここでファイル転送やメッセージ処理へ進む
}

func handleClientMode(pm *peer.PeerManager) {
	// ... クライアントモードの処理
}
