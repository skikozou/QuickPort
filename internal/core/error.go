package core

func IsErrorPacket(packet *BaseData) (*ErrorPacketData, bool) {
	errpac, err := convertMapToErrorPacketData(packet.Data)
	if err != nil {
		return nil, false
	}

	return errpac, true
}

func (h *Handle) SendError(packet *ErrorPacketData, useSub bool) error {
	conn := h.Self.Conn
	addr := h.Peer.Addr
	if useSub {
		conn = h.Self.SubConn
		addr = h.Peer.SubAddr
	}

	return Write(conn, addr.StrAddr(), &BaseData{Type: Error, Data: packet})
}
