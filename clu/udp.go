package clu

import (
	"errors"
	"net"
	"strconv"
	"syscall"
)

// UDP is the production [Transport] (spec §7.1). Each socket binds its own
// ephemeral port. By default it is connected to the CLU, so the kernel
// drops datagrams from any other source. With AnySource it is one
// unconnected socket that sends to the CLU and accepts datagrams from any
// sender on its port: for a CLU with several interfaces that answers from
// another address.
type UDP struct {
	AnySource bool
}

// Open binds the socket and starts its read goroutine.
func (u UDP) Open(host string, port int, r Receiver) (Socket, error) {
	peer, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	s := &udpSocket{peer: peer, any: u.AnySource}
	if u.AnySource {
		s.conn, err = net.ListenUDP("udp", nil)
	} else {
		s.conn, err = net.DialUDP("udp", nil, peer)
	}
	if err != nil {
		return nil, err
	}
	go s.read(r)
	return s, nil
}

type udpSocket struct {
	conn *net.UDPConn
	peer *net.UDPAddr
	any  bool
}

func (s *udpSocket) Send(data []byte) error {
	var err error
	if s.any {
		_, err = s.conn.WriteToUDP(data, s.peer)
	} else {
		_, err = s.conn.Write(data)
	}
	return err
}

func (s *udpSocket) Close() error { return s.conn.Close() }

// read is the socket's only reader.
func (s *udpSocket) read(r Receiver) {
	buf := make([]byte, 65536)
	for {
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// An ICMP "port unreachable" surfaces on a connected socket as
			// a read error; it is not a failure of the socket.
			if errors.Is(err, syscall.ECONNREFUSED) {
				continue
			}
			s.conn.Close()
			r.Closed(err)
			return
		}
		r.Datagram(append([]byte(nil), buf[:n]...))
	}
}
