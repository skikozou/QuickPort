package core

import (
	"QuickPort/enc52"
	"strconv"
	"strings"
)

func GenToken(self *SelfConfig) string {
	ip := string(self.LocalAddr.Ip)
	port := strconv.Itoa(self.LocalAddr.Port)
	name := self.Name

	raw := strings.Join([]string{ip, port, name}, ":")
	token := enc52.Encode(raw)

	return token
}

func ParseToken(token string) (*PeerConfig, error) {
	raw, err := enc52.Decode(token)
	if err != nil {
		return nil, err
	}

	ip := []byte(strings.Split(raw, ":")[0])
	port, err := strconv.Atoi(strings.Split(raw, ":")[1])
	name := strings.Split(raw, ":")[2]

	if err != nil {
		return nil, err
	}

	return &PeerConfig{
		Addr: &Address{
			Ip:   ip,
			Port: port,
		},
		Name: name,
	}, nil
}
