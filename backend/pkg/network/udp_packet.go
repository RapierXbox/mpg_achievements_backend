package network

import "net"

type UDPPacket struct {
	Address *net.UDPAddr
	Payload []byte
}
