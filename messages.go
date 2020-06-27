package main

import (
	"bytes"
	"encoding/binary"
	"io"
)

type OpCode byte

const (
	OpCodeStandard OpCode = 0 + iota
	OpCodeInverse
	OpCodeStatus
)

type ResponseCode byte

const (
	ResponseCodeOk ResponseCode = 0 + iota
	ResponseCodeFormatError
	ResponseCodeServerFailure
	ResponseCodeNameError
	ResponseCodeNotImplemented
	ResponseCodeRefused
)

// TODO: separate Header, etc.
type Message struct {
	ID uint16
	IsResponse bool
	OpCode OpCode
	AuthoritativeAnswer bool
	Truncated bool
	RecursionDesired bool
	RecursionAvailable bool
	ResponseCode ResponseCode
	QdCount uint16
	AnCount uint16
	NSCount uint16
	ARCount uint16
	QName uint16
	Questions []Question
}

type Question struct {
	Name [][]byte
	Type uint16
	Class uint16
}

type Type uint16

const (
	TypeA = 1 + iota
	TypeNS
	TypeMD
	TypeMF
	TypeCName
	TypeSOA
	TypeMB
	TypeMG
	TypeMR
	TypeNull
	TypeWKS
	TypePTR
	TypeHIinfo
	TypeMInfo
	MX
	TXT
)

type Class uint16

const (
	ClassIN = 1 + iota
	ClassCS
	ClassCH
	ClassHS
)

func (m *Message) Marshal() []byte {
	b := make([]byte, 0, 12)

	buf := bytes.NewBuffer(b)
	binary.Write(buf, binary.BigEndian, m.ID)

	var byt byte
	if m.IsResponse {
		byt += 1 << 7
	}
	byt += byte(m.OpCode) << 3
	if m.AuthoritativeAnswer {
		byt += 1 << 2
	}
	if m.Truncated {
		byt += 1 << 1
	}
	if m.RecursionDesired {
		byt += 1
	}

	binary.Write(buf, binary.BigEndian, byt)

	byt = 0

	if m.RecursionAvailable {
		byt += 1 << 7
	}

	byt += byte(m.ResponseCode)

	binary.Write(buf, binary.BigEndian, byt)

	binary.Write(buf, binary.BigEndian, m.QdCount)
	binary.Write(buf, binary.BigEndian, m.AnCount)
	binary.Write(buf, binary.BigEndian, m.NSCount)
	binary.Write(buf, binary.BigEndian, m.ARCount)

	for _, q := range m.Questions {
		for _, label := range q.Name {
			binary.Write(buf, binary.BigEndian, byte(len(label)))
			binary.Write(buf, binary.BigEndian, label)
		}
		binary.Write(buf, binary.BigEndian, q.Type)
		binary.Write(buf, binary.BigEndian, q.Class)
	}

	return buf.Bytes()
}

func Unmarshal(b []byte, m *Message) error {
	// TODO: check if the buffer is the appropriate size
	buf := bytes.NewBuffer(b)
	binary.Read(buf, binary.BigEndian, &m.ID)

	var byt byte
	binary.Read(buf, binary.BigEndian, &byt)
	if byt & 128 == 128 {
		m.IsResponse = true
	}
	// Bug to remember. I forgot to put the >> 3 on here, which was giving me 8 oh no!
	m.OpCode = OpCode(byt & (64 + 32 + 16 + 8)) >> 3
	if byt & 4 == 4 {
		m.AuthoritativeAnswer = true
	}
	if byt & 2 == 2 {
		m.Truncated = true
	}
	if byt & 1 == 1 {
		m.RecursionDesired = true
	}

	binary.Read(buf, binary.BigEndian, &byt)
	if byt & 128 == 128 {
		m.RecursionAvailable = true
	}
	m.ResponseCode = ResponseCode(byt & 15)

	binary.Read(buf, binary.BigEndian, &m.QdCount)
	binary.Read(buf, binary.BigEndian, &m.AnCount)
	binary.Read(buf, binary.BigEndian, &m.NSCount)
	binary.Read(buf, binary.BigEndian, &m.ARCount)

	var qsRead uint16
	for ; qsRead < m.QdCount; qsRead++ {
		m.Questions = append(m.Questions, decodeQuestion(buf))
	}

	return nil
}

func decodeQuestion(buf io.Reader) Question {
	q := Question{}

	for {
		var labelLen byte
		binary.Read(buf, binary.BigEndian, &labelLen)
		if labelLen == 0 {
			break
		}
		label := make([]byte, labelLen)
		buf.Read(label)
		q.Name = append(q.Name, label)
	}
	binary.Read(buf, binary.BigEndian, &q.Type)
	binary.Read(buf, binary.BigEndian, &q.Class)

	return q
}