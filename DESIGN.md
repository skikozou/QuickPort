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

### 5.1 基本パケット構造

#### チャンクパケット構造
```
[パケットID(8バイト)][チャンクインデックス(4バイト)][チャンクサイズ(4バイト)][チェックサム(4バイト)][チャンク(可変長)]
```

- パケットID: uint64 (8バイト) - ビッグエンディアン
- チャンクインデックス: int32 (4バイト) - ビッグエンディアン
  - 単一チャンクの場合は-1
  - 複数チャンクの場合は0からの連番
- チャンクサイズ: uint32 (4バイト) - ビッグエンディアン
- チェックサム: uint32 (4バイト) - ビッグエンディアン
- チャンク: 可変長バイト列
  - 基本的にJSON形式のデータ

#### 初期チャンク情報パケット構造
```
[パケットID(8バイト)][チャンク数(4バイト)][総サイズ(8バイト)]
```

- パケットID: uint64 (8バイト) - ビッグエンディアン
- チャンク数: uint32 (4バイト) - ビッグエンディアン
- 総サイズ: uint64 (8バイト) - ビッグエンディアン

### 5.2 パケット処理フロー

1. 初期接続時の最大パケットサイズ測定
   ```sequence
   Host -> Client: テストパケット(サイズ段階的に増加)
   Client -> Host: 受信確認
   Host -> Client: 最適なチャンクサイズを決定
   ```

2. 大きなデータの送信フロー
   ```sequence
   Sender -> Receiver: 初期チャンク情報パケット
   Receiver -> Sender: 受信準備完了
   Sender -> Receiver: チャンクパケット(1)
   Sender -> Receiver: チャンクパケット(2)
   ...
   Sender -> Receiver: チャンクパケット(n)
   Receiver -> Sender: 全チャンク受信完了
   ```

### 5.3 チャンクサイズ決定アルゴリズム

1. 初期接続時の処理
   - 開始サイズ: 512バイト
   - 最大試行サイズ: 65507バイト (UDP最大ペイロード)
   - 段階的にサイズを増やしながらテストパケットを送信
   - 安定して受信できる最大サイズの80%をチャンクサイズとして採用

2. チャンクサイズの保存
   - セッション中は決定したチャンクサイズを維持
   - 必要に応じて再測定可能

### 5.4 データ型定義

```go
// パケットヘッダー構造体
type PacketHeader struct {
    ID            uint64  // パケットID
    ChunkIndex    int32   // チャンクインデックス
    ChunkSize     uint32  // チャンクサイズ
    Checksum      uint32  // チェックサム
}

// 初期チャンク情報構造体
type InitialChunkInfo struct {
    ID            uint64  // パケットID
    ChunkCount    uint32  // チャンク数
    TotalSize     uint64  // 総サイズ
}
```

### 5.5 チェックサム計算

- チェックサム対象: チャンクデータのみ
- アルゴリズム: CRC32
- 検証: チャンク受信時にチェックサム再計算で整合性確認

### 5.6 エラー処理

1. チャンク欠損時
   - 欠損チャンクのインデックスを記録
   - 再送要求パケットで欠損チャンクのみ再送

2. チェックサムエラー時
   - 該当チャンクの再送要求
   - 連続エラー時はチャンクサイズの再調整を検討

3. パケットID不整合
   - 不正なシーケンスを検出
   - セッションの再確立を検討

### 5.7 最適化戦略

1. チャンクバッファリング
   - 送信側: チャンク送信キューの実装
   - 受信側: チャンク結合バッファの実装

2. パイプライン処理
   - 次のチャンク準備と現在のチャンク送信の並行処理
   - チャンク結合処理の非同期実行

3. フロー制御
   - 受信バッファの状態に応じた送信レート調整
   - チャンク受信確認の頻度調整

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

## 12. 接続確立フロー

### 12.1 トークン構造

```go
// トークンデータ構造
type TokenData struct {
    Version    uint8     // トークンバージョン
    Name       string    // ホスト名
    IP         net.IP    // ホストのIPアドレス
    Port       uint16    // ホストのUDPポート
    Timestamp  int64     // トークン生成時のUNIXタイムスタンプ
    Salt       []byte    // ランダムソルト (16バイト)
}
```

トークン生成手順：
1. TokenDataをJSONにシリアライズ
2. データを圧縮（zlib）
3. enc52エンコード

### 12.2 接続シーケンス

```sequence
Host -> Host: トークン生成
Client -> Client: トークンデコード
Client -> Host: 接続リクエスト
Host -> Client: 接続応答 (Accept/Reject)
Client -> Host: 接続確認
Host -> Client: チャンクサイズ測定開始
Host -> Client: テストパケット送信
Client -> Host: 測定結果送信
Host -> Client: セッション確立完了
```

### 12.3 パケットタイプ（接続フェーズ）

```go
const (
    // 接続関連パケット
    PacketTypeConnectionRequest  uint8 = 1
    PacketTypeConnectionResponse uint8 = 2
    PacketTypeConnectionConfirm  uint8 = 3
    PacketTypeChunkSizeTest     uint8 = 4
    PacketTypeChunkSizeResult   uint8 = 5
    PacketTypeSessionReady      uint8 = 6
)

// 接続リクエストデータ
type ConnectionRequest struct {
    ClientName   string    // クライアント名
    ClientPort   uint16    // クライアントのポート
    TokenHash    []byte    // トークンの検証用ハッシュ
}

// 接続レスポンスデータ
type ConnectionResponse struct {
    Status      uint8     // 0: 拒否, 1: 許可
    Reason      string    // 拒否理由（拒否時のみ）
    SessionID   uint64    // セッションID（許可時のみ）
}
```

### 12.4 接続フロー詳細

#### 1. トークン生成（ホスト側）
```go
func GenerateToken() string {
    tokenData := TokenData{
        Version:    1,
        Name:      "HostName",
        IP:        localIP,
        Port:      localPort,
        Timestamp: time.Now().Unix(),
        Salt:      generateRandomSalt(16),
    }
    // シリアライズ、圧縮、エンコード処理
}
```

#### 2. トークンデコード（クライアント側）
- enc52デコード
- 解凍
- JSONデシリアライズ
- タイムスタンプ検証（有効期限10分）

#### 3. 接続リクエスト送信（クライアント側）
- ホストのIP:Portに接続リクエストパケット送信
- トークンハッシュを含めて送信（検証用）
- 最大3回まで再試行（3秒間隔）

#### 4. 接続応答処理（ホスト側）
- トークンハッシュ検証
- クライアント情報の検証
- セッションID生成
- 応答送信

#### 5. チャンクサイズ測定
- 段階的なサイズでテストパケット送信
- 往復時間（RTT）計測
- パケットロス率計測
- 最適なチャンクサイズ決定

### 12.5 エラーケース

1. トークン無効
```sequence
Client -> Host: 接続リクエスト（無効なトークン）
Host -> Client: 拒否応答（理由: 無効なトークン）
```

2. タイムアウト
```sequence
Client -> Host: 接続リクエスト
Client -> Client: タイムアウト（3秒）
Client -> Host: 接続リクエスト（再試行1）
```

3. チャンクサイズ測定失敗
```sequence
Host -> Client: テストパケット
Client -> Host: 測定失敗通知
Host -> Client: 最小サイズでの再測定開始
```

### 12.6 セキュリティ考慮事項

1. トークン有効期限
- 生成から10分間のみ有効
- タイムスタンプチェックで制御

2. トークン検証
- ソルトによる予測防止
- ハッシュによる改ざん検知

3. 接続制限
- 同時接続数の制限（1接続のみ）
- IP単位の接続試行回数制限

## 13. 入出力管理

### 13.1 ロギング (Output)
- パッケージ: github.com/sirupsen/logrus
- ログレベル:
  ```go
  logrus.ErrorLevel   // エラー（接続失敗など）
  logrus.WarnLevel    // 警告（再試行など）
  logrus.InfoLevel    // 情報（接続確立など）
  logrus.DebugLevel   // デバッグ情報
  ```
- フォーマット設定:
  ```go
  logrus.SetFormatter(&logrus.TextFormatter{
      FullTimestamp:   true,
      TimestampFormat: "2006-01-02 15:04:05",
  })
  ```

### 13.2 ユーザー入力 (Input)
- パッケージ: github.com/AlecAivazis/survey/v2
- スタイル設定:
  ```go
  // グローバルスタイル定義
  surveyCore.SetTheme(&surveyCore.Theme{
      Question:  &surveyCore.Styler{FgCyan: true},
      Help:      &surveyCore.Styler{FgBlue: true},
      Error:     &surveyCore.Styler{FgRed: true, Bold: true},
      SelectFocus: &surveyCore.Styler{FgCyan: true},
  })
  ```

#### 13.2.1 入力タイプ

1. コマンド選択メニュー
```go
var commandPrompt = &survey.Select{
    Message: "選択してください:",
    Options: []string{
        "ファイル送信",
        "メッセージ送信",
        "状態確認",
        "終了",
    },
    Help: "↑↓で選択、Enterで決定",
}
```

2. ファイル選択
```go
var filePrompt = &survey.Input{
    Message: "ファイルパス:",
    Help: "送信したいファイルのパスを入力",
    Suggest: func(toComplete string) []string {
        // ファイルパスの補完候補を提供
        return getFileSuggestions(toComplete)
    },
}
```

3. メッセージ入力
```go
var messagePrompt = &survey.Input{
    Message: "メッセージ:",
    Help: "送信したいメッセージを入力",
}
```

4. 確認ダイアログ
```go
var confirmPrompt = &survey.Confirm{
    Message: "続行しますか?",
    Help: "Y/N で選択",
}
```

#### 13.2.2 入力バリデーション
```go
var validators = map[string]*survey.InputValidator{
    "token": survey.ComposeValidators(
        survey.Required,
        survey.MinLength(8),
    ),
    "filepath": survey.ComposeValidators(
        survey.Required,
        survey.PathExists,
    ),
}
```

### 13.3 入出力インターフェース

```go
// IO管理インターフェース
type IOManager interface {
    // 出力関連（logrus）
    Info(format string, args ...interface{})
    Error(format string, args ...interface{})
    Debug(format string, args ...interface{})
    Warning(format string, args ...interface{})
    
    // 入力関連（survey）
    AskCommand() (string, error)
    AskToken() (string, error)
    AskFilePath() (string, error)
    AskMessage() (string, error)
    Confirm(message string) (bool, error)
}

// 実装例
type DefaultIOManager struct {
    logger *logrus.Logger
}
```

### 13.4 入力処理の具体例

#### トークン入力
```go
func (m *DefaultIOManager) AskToken() (string, error) {
    token := ""
    prompt := &survey.Input{
        Message: "接続トークンを入力してください:",
        Help:    "相手から受け取ったトークンを入力",
    }
    
    err := survey.AskOne(prompt, &token, survey.WithValidator(validators["token"]))
    return token, err
}
```

#### ファイル送信フロー
```go
func (m *DefaultIOManager) HandleFileSend() error {
    // ファイルパス入力
    filePath := ""
    err := survey.AskOne(filePrompt, &filePath, survey.WithValidator(validators["filepath"]))
    if err != nil {
        return err
    }
    
    // 確認
    confirm := false
    confirmMsg := fmt.Sprintf("'%s' を送信しますか?", filePath)
    err = survey.AskOne(&survey.Confirm{Message: confirmMsg}, &confirm)
    if err != nil || !confirm {
        return err
    }
    
    // 送信処理...
    return nil
}
```

### 13.5 エラーハンドリング

```go
// エラー種別
var (
    ErrInterrupt = errors.New("operation interrupted")
    ErrInvalid   = errors.New("invalid input")
)

// エラー処理例
func handleInputError(err error) {
    switch {
    case errors.Is(err, ErrInterrupt):
        logrus.Info("操作がキャンセルされました")
    case errors.Is(err, ErrInvalid):
        logrus.Error("無効な入力です")
    default:
        logrus.Errorf("入力エラー: %v", err)
    }
}
```

### 13.6 プログレス表示

```go
func showProgress(total int) {
    prompt := &survey.Progress{
        Message: "送信中...",
        Total:   total,
    }
    
    for i := 0; i < total; i++ {
        prompt.Increment()
        time.Sleep(time.Millisecond * 100)
    }
}
```