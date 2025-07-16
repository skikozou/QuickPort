# QuickPort 設計ドキュメント

## 1. 概要

QuickPortは、シンプルなP2P通信を実現するGoアプリケーションです。UDP通信を利用して、ファイル転送とメッセージングを提供します。

## 2. システム構造

```
QuickPort/
├── cmd/
│   └── quickport/
│       └── main.go          # エントリーポイント
├── internal/
│   ├── network/            # 通信関連
│   │   ├── connection.go   # UDP接続管理
│   │   └── packet.go      # パケット定義と処理
│   ├── peer/              # ピア関連
│   │   ├── manager.go     # ピア管理
│   │   └── token.go       # 接続トークン処理
│   ├── transfer/          # 転送関連
│   │   ├── file.go        # ファイル転送
│   │   └── message.go     # メッセージング
│   └── core/              # コア機能
│       ├── config.go      # 設定管理
│       └── session.go     # セッション管理
└── pkg/
    └── utils/             # ユーティリティ
        ├── logger.go      # ログ機能
        └── common.go      # 共通機能
```

## 3. 各ファイルの役割

### 3.1 cmd/quickport/main.go
- アプリケーションのエントリーポイント
- コマンドライン引数の処理
- セッションの初期化と実行

### 3.2 internal/network/
#### connection.go
- UDP接続の確立と管理
- パケットの送受信処理
- 接続状態の管理

#### packet.go
- パケット構造の定義
- パケットのシリアライズ/デシリアライズ
- パケットタイプの管理

### 3.3 internal/peer/
#### manager.go
- ピア情報の管理
- ピアの状態監視
- ピアとの接続管理

#### token.go
- 接続トークンの生成
- トークンの検証と解析
- セキュリティ関連の処理

### 3.4 internal/transfer/
#### file.go
- ファイル転送の制御
- チャンク分割と結合
- 転送進捗の管理

#### message.go
- メッセージングシステム
- メッセージの送受信
- メッセージキューの管理

### 3.5 internal/core/
#### config.go
- アプリケーション設定の管理
- 設定値の検証
- 環境変数の処理

#### session.go
- セッション状態の管理
- 通信フローの制御
- エラーハンドリング

## 4. 通信の流れ

### 4.1 セッション確立
1. ホストがトークンを生成
2. クライアントがトークンを入力
3. UDP接続の確立
4. 初期ハンドシェイク

```sequence
Host -> Token: 生成
Client -> Token: 入力
Client -> Host: 接続要求
Host -> Client: 承認
```

### 4.2 ファイル転送
1. 送信側がファイル情報を送信
2. 受信側が受信準備完了を通知
3. チャンク単位でのファイル転送
4. 転送完了確認

### 4.3 メッセージング
1. メッセージパケットの作成
2. 直接UDP送信
3. 受信確認（オプション）

## 5. パケット構造

```go
type PacketType uint8

const (
    PacketTypeMessage PacketType = iota
    PacketTypeFile
    PacketTypeControl
)

type Packet struct {
    Type      PacketType       
    Timestamp time.Time        
    Data      json.RawMessage 
}
```

## 6. ユーザー入力の管理

### 6.1 コマンドライン引数
- `--port`: ローカルポート番号（省略時はランダム）
- `--token`: 接続トークン（省略時はホストモード）

### 6.2 対話的コマンド
```
/send <filename>   # ファイル送信
/msg <message>     # メッセージ送信
/status           # 接続状態確認
/quit             # 終了
```

## 7. エラーハンドリング

### 7.1 ネットワークエラー
- 接続タイムアウト
- パケットロス
- 不正なパケット

### 7.2 ファイル転送エラー
- ファイルアクセスエラー
- 容量不足
- チャンク欠損

### 7.3 リカバリー処理
- 再送制御
- セッション再確立
- エラーログ

## 8. 設定パラメータ

```go
type Config struct {
    Network struct {
        Port            int
        BufferSize      int
        Timeout        time.Duration
    }
    Transfer struct {
        ChunkSize      int
        MaxRetries     int
        RetryInterval time.Duration
    }
}
```

## 9. 今後の拡張可能性

1. ファイル転送の暗号化
2. 複数ピアとの同時接続
3. ファイル転送の一時停止/再開
4. GUI インターフェース
5. 転送速度の最適化

## 10. 制限事項

1. 最大ファイルサイズ: 2GB
2. 同時接続数: 1
3. サポートするプラットフォーム: Linux, macOS, Windows
4. 必要なポート: 1つ（UDP）

## 11. デプロイメント

### 11.1 ビルド
```bash
go build -o quickport ./cmd/quickport
```

### 11.2 実行
```bash
# ホストモード
./quickport --port 8080

# クライアントモード
./quickport --token <接続トークン>
```