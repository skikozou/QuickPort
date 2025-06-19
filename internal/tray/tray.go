package tray

import (
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

var trayPath string

func UseTray() string {
	return trayPath
}

func SetTray(path string) error {
	traypath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	trayPath = traypath + "\\"
	return nil
}

func GetTrayItems(dir string) ([]FileMeta, error) {
	files := []FileMeta{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			file, err := NewFileMeta(path, dir)
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

func NewFileMeta(path string, baseDir string) (*FileMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	hash, err := HashFile(path)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		return nil, err
	}

	return &FileMeta{
		Filename: relPath,
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
