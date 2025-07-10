package core

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

func Compress(raw []byte, mode string) ([]byte, error) {
	switch mode {
	case "high":
		// zstd (level 19)

		encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
		if err != nil {
			return nil, err
		}
		defer encoder.Close()
		return encoder.EncodeAll(raw, make([]byte, 0, len(raw))), nil

	case "medium":
		// gzip (BestSpeed)

		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err != nil {
			return nil, err
		}
		_, err = w.Write(raw)
		if err != nil {
			return nil, err
		}
		w.Close()
		return buf.Bytes(), nil

	case "low":
		// snappy

		return snappy.Encode(nil, raw), nil

	case "none":
		// none

		return raw, nil

	default:
		// medium or none

		return raw, nil

	}
}

func Decompress(data []byte, mode string) ([]byte, error) {
	switch mode {
	case "high":
		// zstd
		decoder, err := zstd.NewReader(nil, zstd.WithDecoderMaxWindow(512<<20)) // 最大512MiBまで許容
		if err != nil {
			return nil, err
		}
		defer decoder.Close()
		return decoder.DecodeAll(data, nil)

	case "medium":
		// gzip
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)

	case "low":
		// snappy
		return snappy.Decode(nil, data)

	case "none":
		// none
		return data, nil

	default:
		// none or medium
		return data, nil
	}
}
