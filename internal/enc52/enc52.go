package enc52

import (
	"fmt"
	"strings"
)

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func Encode(data string) string {
	var sb strings.Builder
	for _, b := range []byte(data) {
		high := b / 52
		low := b % 52
		sb.WriteByte(alphabet[high])
		sb.WriteByte(alphabet[low])
	}
	return sb.String()
}

func Decode(encoded string) (string, error) {
	if len(encoded)%2 != 0 {
		return "", fmt.Errorf("invalid encoded length")
	}
	decoded := make([]byte, len(encoded)/2)
	for i := 0; i < len(encoded); i += 2 {
		high := strings.IndexByte(alphabet, encoded[i])
		low := strings.IndexByte(alphabet, encoded[i+1])
		if high < 0 || low < 0 {
			return "", fmt.Errorf("invalid character")
		}
		decoded[i/2] = byte(high*52 + low)
	}
	return string(decoded), nil
}

//high*52 + low
