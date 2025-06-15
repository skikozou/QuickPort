package tray

import (
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
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
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	h := fnv.New32a()
	h.Write([]byte(raw))
	return strconv.FormatUint(uint64(h.Sum32()), 10), nil
}
