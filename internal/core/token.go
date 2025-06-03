package core

import (
	"QuickPort/enc52"
	"strconv"
	"strings"
)

func GenToken(self *Self) string {
	ip := string(self.GlobalAddr.Ip)
	port := strconv.Itoa(self.GlobalAddr.Port)
	name := self.Name

	raw := strings.Join([]string{ip, port, name}, ":")
	token := enc52.Encode(raw)

	return token
}

func ParseToken(token string) (*Peer, error) {
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

	return &Peer{
		Addr: &Address{
			Ip:   ip,
			Port: port,
		},
		Name: name,
	}, nil
}
