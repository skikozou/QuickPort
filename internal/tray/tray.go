package tray

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func GetTrayItems(dir string) ([]FileMeta, error) {
	files := []FileMeta{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			file, err := NewFileMeta(path)
			if err != nil {
				return nil
			}

			files = append(files, *file)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

func NewFileMeta(path string) (*FileMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	hash, err := HashFile(path) // 下に定義
	if err != nil {
		return nil, err
	}

	return &FileMeta{
		Filename: info.Name(),
		Size:     info.Size(),
		Hash:     hash,
	}, nil
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
