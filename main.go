package main

import (
	"fmt"
	"log"
	"net"
)

const maxBufferSize = 2048

type Client struct {
	addr *net.UDPAddr
}

func NewClient(hostPort string) (*Client, error) {
	addr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return nil, fmt.Errorf("resolving addr: %v", err)
	}
	return &Client{
		addr: addr,
	}, nil
}

func (cli *Client) Resolve(q Message) (Message, error) {
	m := Message{}
	conn, err := net.DialUDP("udp", nil, cli.addr)
	if err != nil {
		return m, fmt.Errorf("dialing upstream DNS server: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write(q.Marshal()); err != nil {
		return m, fmt.Errorf("writing message: %v", err)
	}
	buf := make([]byte, maxBufferSize)
	if _, _, err := conn.ReadFrom(buf); err != nil {
		return m, fmt.Errorf("reading response: %v", err)
	}
	Unmarshal(buf, &m)
	return m, nil
}

type Server struct {
	conn net.PacketConn
	cli  *Client
}

func NewServer(port string, cli *Client) (*Server, error) {
	conn, err := net.ListenPacket("udp", "localhost:"+port)
	if err != nil {
		return nil, err
	}
	return &Server{
		conn: conn,
		cli:  cli,
	}, nil
}

func (s *Server) Listen() error {
	buff := make([]byte, maxBufferSize)
	for {
		_, addr, err := s.conn.ReadFrom(buff)
		if err != nil {
			log.Println("error reading from conn", err)
			continue
		}
		q := Message{}
		Unmarshal(buff, &q)
		ans, err := s.cli.Resolve(q)
		if err != nil {
			log.Println(err)
		}
		if _, err := s.conn.WriteTo(ans.Marshal(), addr); err != nil {
			log.Println("error writing to conn:", err)
		}
	}
	return nil
}

func (s *Server) Close() error {
	return s.conn.Close()
}

func main() {
	cli, err := NewClient("8.8.8.8:53")
	if err != nil {
		log.Fatal(err)
	}

	server, err := NewServer("5003", cli)
	if err != nil {
		log.Fatal(err)
	}

	if err := server.Listen(); err != nil {
		log.Fatal(err)
	}
}
