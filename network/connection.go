package network

import (
	"fmt"
	"net"
	"time"
)

func NewConnection(config ConnectionConfig) (*Connection, error) {
	conn := &Connection{
		config: config,
		isHost: config.LocalPort != 0,
	}

	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: config.LocalPort,
	}

	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	conn.conn = udpConn
	return conn, nil
}

func (c *Connection) Send(data []byte) error {
	if c.remoteAddr == nil {
		return fmt.Errorf("no remote address set")
	}

	_, err := c.conn.WriteToUDP(data, c.remoteAddr)
	return err
}

func (c *Connection) Receive(timeout time.Duration) ([]byte, *net.UDPAddr, error) {
	if timeout > 0 {
		c.conn.SetReadDeadline(time.Now().Add(timeout))
	}

	buffer := make([]byte, c.config.BufferSize)
	n, addr, err := c.conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, nil, err
	}

	return buffer[:n], addr, nil
}

func (c *Connection) Close() error {
	return c.conn.Close()
}

func (c *Connection) SetRemoteAddr(addr *net.UDPAddr) {
	c.remoteAddr = addr
}

func (c *Connection) LocalPort() int {
	if c.localAddr != nil {
		return c.localAddr.Port
	}
	return 0
}
