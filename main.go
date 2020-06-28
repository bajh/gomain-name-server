package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"reflect"
	"strings"
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

func (cli *Client) Resolve(q Question) (Message, error) {
	m := Message{
		ID:               uint16(rand.Intn(math.MaxUint16)),
		OpCode:           OpCodeStandard,
		RecursionDesired: true,
		QdCount:          1,
		Questions:        []Question{q},
	}
	conn, err := net.DialUDP("udp", nil, cli.addr)
	if err != nil {
		return m, fmt.Errorf("dialing upstream DNS server: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write(m.Marshal()); err != nil {
		return m, fmt.Errorf("writing message: %v", err)
	}
	buf := make([]byte, maxBufferSize)
	// TODO: we have to match this up with the packet that was sent because there might be multiple in flight
	if _, _, err := conn.ReadFrom(buf); err != nil {
		return m, fmt.Errorf("reading response: %v", err)
	}
	Unmarshal(buf, &m)
	return m, nil
}

func domainSuffixLen(a [][]byte, b [][]byte) int {
	var suffixLen int
	aPtr := len(a) - 1
	bPtr := len(b) - 1
	for aPtr >= 0 && bPtr >= 0 {
		if bytes.Equal(a[aPtr], b[bPtr]) {
			suffixLen += 1
		} else {
			break
		}
		aPtr--
		bPtr--
	}
	return suffixLen
}

func toIPStr(data []byte) string {
	b := make([]string, 4)
	for i, byt := range data {
		b[i] = fmt.Sprintf("%d", byt)
	}
	return strings.Join(b, ".")
}

func (cli *Client) ResolveRecursively(q Question) (Message, error) {
	// Ask for domain name
	m, err := cli.Resolve(q)
	if err != nil {
		return m, fmt.Errorf("attempting to resolve name: %v", err)
	}
	// If we get an A record, that matches our query, return it
	// If we get an NS record that partially or fully matches our query, check if we also have an A record for it. If we don't, we have to resolve that name
	var ns *ResourceRecord
	var bestSuffixLen int
	for _, ans := range m.Answer {
		if ans.Type == TypeA && reflect.DeepEqual(q.Name, ans.Name) {
			return m, nil
		}
		// TODO: this is actually supposed to come from the Authority section I think?
		if ans.Type == TypeNS {
			suffixLen := domainSuffixLen(q.Name, ans.Name)
			if suffixLen > bestSuffixLen {
				ns = &ans
				bestSuffixLen = suffixLen
			}
		}
	}

	if ns != nil {
		//var nsARecord *ResourceRecord
		//// I think this is actually supposed to come from the Additional section?
		//for _, ans := range m.Answer {
		//	if ans.Type == TypeA && reflect.DeepEqual(ns.Data, ans.Name) {
		//		nsARecord = &ans
		//		break
		//	}
		//}
		//if nsARecord == nil {
		//	// resolve recursively if this happens
		//	return m, fmt.Errorf("I'd have to resolve this NS recursively, and I'm not willing to do that")
		//}

		nsCli, err := NewClient(toIPStr(ns.Data) + ":53")
		if err != nil {
			return m, fmt.Errorf("creating nameserver client: %v", err)
		}
		return nsCli.ResolveRecursively(q)
	}

	return m, fmt.Errorf("dead end")
}

// Test for Implementing recursion
// Create a server that's just hardcoded to send us to Google for com:
// * (com, dns.google.com, NS, IN)
// * (dns.google.com, 8.8.8.8, A, IN)
// This tells us we should go to that server to resolve our com addresses
// We'll have to keep doing this until we get back an answer to our real question

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

func (s *Server) ListenAsUpstream() error {
	buff := make([]byte, maxBufferSize)
	for {
		_, addr, err := s.conn.ReadFrom(buff)
		if err != nil {
			log.Println("error reading from conn", err)
			continue
		}
		m := Message{}
		Unmarshal(buff, &m)
		ans := Message{
			ID:                 m.ID,
			IsResponse:         true,
			OpCode:             m.OpCode,
			RecursionAvailable: false,
			ResponseCode:       ResponseCodeOk,
			QdCount:            1,
			AnCount:            2,
			Questions:          m.Questions,
			Answer: []ResourceRecord{
				{
					Name:  [][]byte{[]byte("com")},
					Type:  TypeNS,
					Class: ClassIN,
					// I'm not sure what the Data section of an NS type is supposed to look like, come back to this
					// Data:  []byte("dns.google.com"),
					Data: []byte{8, 8, 8, 8},
				},
				{
					Name:  [][]byte{[]byte("dns"), []byte("google"), []byte("com")},
					Type:  TypeA,
					Class: ClassIN,
					Data:  []byte{8, 8, 8, 8},
				},
			},
		}
		if _, err := s.conn.WriteTo(ans.Marshal(), addr); err != nil {
			log.Println("error writing to conn:", err)
		}
	}
	return nil
}

func (s *Server) Listen() error {
	buff := make([]byte, maxBufferSize)
	// TODO: be able to handle multiple connections at a time
	for {
		_, addr, err := s.conn.ReadFrom(buff)
		if err != nil {
			log.Println("error reading from conn", err)
			continue
		}
		m := Message{}
		Unmarshal(buff, &m)
		// Not sure how to deal with multiple questions atm. And if there are no questions I'm pretty sure the prescribed behavior is for the server to crash
		upstreamAns, err := s.cli.ResolveRecursively(m.Questions[0])
		if err != nil {
			log.Println(err)
			continue
		}
		ans := Message{
			ID:                 m.ID,
			IsResponse:         true,
			OpCode:             OpCodeStandard,
			RecursionAvailable: true,
			ResponseCode:       ResponseCodeOk,
			QdCount:            m.QdCount,
			Questions:          m.Questions,
			AnCount:            upstreamAns.AnCount,
			Answer:             upstreamAns.Answer,
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
	upstream := flag.Bool("upstream", false, "act as an upstream")
	flag.Parse()
	if *upstream {
		cli, err := NewClient("8.8.8.8:53")
		if err != nil {
			log.Fatal(err)
		}

		server, err := NewServer("5005", cli)
		if err != nil {
			log.Fatal(err)
		}

		if err := server.ListenAsUpstream(); err != nil {
			log.Fatal(err)
		}
		return
	}

	cli, err := NewClient("localhost:5005")
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
