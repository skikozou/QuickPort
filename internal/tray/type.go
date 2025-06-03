package tray

type FileMeta struct {
	Filename string `json:"filename"` // ファイル名
	Size     int64  `json:"size"`     // バイト数
	Hash     string `json:"hash"`     // 簡易整合性確認用（例: SHA256）
}
