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
	Questions []Question
	Answer []ResourceRecord
	Authority []ResourceRecord
	Additional []ResourceRecord
}

type Question struct {
	Name [][]byte
	Type Type
	Class Class
}

type ResourceRecord struct {
	Name [][]byte
	Type Type
	Class Class
	TTL uint32
	Data []byte
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
		binary.Write(buf, binary.BigEndian, byte(0))
		binary.Write(buf, binary.BigEndian, q.Type)
		binary.Write(buf, binary.BigEndian, q.Class)
	}

	for _, rr := range m.Answer {
		encodeResourceRecord(buf, rr)
	}
	for _, rr := range m.Authority {
		encodeResourceRecord(buf, rr)
	}
	for _, rr := range m.Additional {
		encodeResourceRecord(buf, rr)
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
	offset := 12 // skip over header
	for ; qsRead < m.QdCount; qsRead++ {
		q, n := decodeQuestion(buf)
		// A bug happened because I forgot to set the offset of the ResourceScanner to 12 +
		// the question section length
		offset += n
		m.Questions = append(m.Questions, q)
	}

	scanner := NewResourceRecordScanner(b, offset)
	var ansRead uint16
	for ; ansRead < m.AnCount; ansRead++ {
		m.Answer = append(m.Answer, scanner.decodeRecord())
	}

	var nsRead uint16
	for ; nsRead < m.NSCount; nsRead++ {
		m.Authority = append(m.Authority, scanner.decodeRecord())
	}

	var arRead uint16
	for ; arRead < m.ARCount; arRead++ {
		m.Additional = append(m.Additional, scanner.decodeRecord())
	}

	return nil
}

func decodeQuestion(buf io.Reader) (Question, int) {
	q := Question{}
	n := 0

	for {
		var labelLen byte
		binary.Read(buf, binary.BigEndian, &labelLen)
		if labelLen == 0 {
			n += 1
			break
		}
		label := make([]byte, labelLen)
		buf.Read(label)
		q.Name = append(q.Name, label)
		n += int(labelLen + 1)
	}
	binary.Read(buf, binary.BigEndian, &q.Type)
	binary.Read(buf, binary.BigEndian, &q.Class)
	n += 4

	return q, n
}

func NewResourceRecordScanner(buf []byte, pos int) *ResourceRecordScanner {
	return &ResourceRecordScanner{
		buf: buf,
		pos: pos,
	}
}

type ResourceRecordScanner struct {
	buf []byte
	pos int
}

// Options for how to deal with pointers:
// * Parse every resource record, leaving unresolved pointers in the parsed record.
// Index every record by the offset, then go through and resolve each pointer
// * Every time we encounter a pointer, seek to that offset and decode it, then append its information
// to the current record
// * Combination: every time we encounter a pointer, seek to that offset and decode it, then append
// its information to the current record. Also keep a cache of the offset -> records we've read so far
// and whenever we encounter an offset, check if we've already parsed it

// A bug! I initially figured I could just implement the pointer stuff later, but no such luck of course
func (r *ResourceRecordScanner) decodeRecord() ResourceRecord {
	rr := ResourceRecord{}
	startOffset := r.pos
	recordLen := 0

	labels, scanned := r.scanLabelsAt(startOffset)
	rr.Name = labels

	buf := bytes.NewBuffer(r.buf[startOffset + scanned:])
	recordLen += scanned

	binary.Read(buf, binary.BigEndian, &rr.Type)
	recordLen += 2
	binary.Read(buf, binary.BigEndian, &rr.Class)
	recordLen += 2
	binary.Read(buf, binary.BigEndian, &rr.TTL)
	recordLen += 4
	var dataLen uint16
	binary.Read(buf, binary.BigEndian, &dataLen)
	data := make([]byte, dataLen)
	buf.Read(data)
	rr.Data = data
	recordLen += int(dataLen + 2)
	r.pos += recordLen
	return rr
}

func (r *ResourceRecordScanner) scanLabelsAt(startOffset int) ([][]byte, int) {
	buf := bytes.NewBuffer(r.buf[startOffset:])
	var labels [][]byte
	var scanned int

	for {
		var labelLen byte
		binary.Read(buf, binary.BigEndian, &labelLen)
		// termination of labels
		if labelLen == 0 {
			return labels, scanned + 1
		}
		if labelLen & 192 == 192 {
			// scanLabelsAt this position
			var dereferencingOffset uint16 = uint16(labelLen & 63) << 7
			var offsetSecondByte byte
			binary.Read(buf, binary.BigEndian, &offsetSecondByte)
			dereferencingOffset += uint16(offsetSecondByte)
			ptrLabels, _ := r.scanLabelsAt(int(dereferencingOffset))
			return append(labels, ptrLabels...), scanned + 2
		}
		label := make([]byte, labelLen)
		buf.Read(label)
		labels = append(labels, label)
		scanned += int(labelLen) + 1
	}
}

func encodeResourceRecord(buf io.Writer, rr ResourceRecord) {
	for _, label := range rr.Name {
		binary.Write(buf, binary.BigEndian, byte(len(label)))
		binary.Write(buf, binary.BigEndian, label)
	}
	binary.Write(buf, binary.BigEndian, byte(0))

	binary.Write(buf, binary.BigEndian, rr.Type)
	binary.Write(buf, binary.BigEndian, rr.Class)
	binary.Write(buf, binary.BigEndian, rr.TTL)
	binary.Write(buf, binary.BigEndian, uint16(len(rr.Data)))
	buf.Write(rr.Data)
}