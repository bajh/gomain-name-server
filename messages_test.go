package main

import (
	"log"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMarshal(t *testing.T) {
	type Test struct {
		Description string
		Message     Message
		Expected    []byte
	}

	tests := []Test{
		{
			Description: "A message with all fields",
			Message: Message{
				ID:                  300,
				IsResponse:          true,
				OpCode:              OpCodeInverse,
				AuthoritativeAnswer: true,
				Truncated:           true,
				RecursionDesired:    true,
				RecursionAvailable:  true,
				ResponseCode:        ResponseCodeRefused,
				QdCount:             257,
				AnCount:             2,
				NSCount:             65535,
				ARCount:             512,
			},
			Expected: []byte{
				1, 44, 143, 133, 1, 1, 0, 2, 255, 255, 2, 0,
			},
		},
	}
	for _, test := range tests {
		buf := test.Message.Marshal()
		log.Println(len(buf))
		for j, b := range buf {
			if test.Expected[j] != b {
				t.Errorf("%s mismatch: expected %v, got %v", test.Description, test.Expected, buf)
				break
			}
		}
	}
}

func TestUnmarshal(t *testing.T) {
	type Test struct {
		Description string
		Bytes       []byte
		Expected    Message
		ExpectedErr error
	}

	tests := []Test{
		{
			Description: "A message with all fields",
			Bytes:       []byte{1, 44, 143, 133, 1, 1, 0, 2, 255, 255, 2, 0},
			Expected: Message{
				ID:                  300,
				IsResponse:          true,
				OpCode:              OpCodeInverse,
				AuthoritativeAnswer: true,
				Truncated:           true,
				RecursionDesired:    true,
				RecursionAvailable:  true,
				ResponseCode:        ResponseCodeRefused,
				QdCount:             257,
				AnCount:             2,
				NSCount:             65535,
				ARCount:             512,
			},
		},
	}
	for _, test := range tests {
		m := Message{}
		Unmarshal(test.Bytes, &m)
		if !cmp.Equal(test.Expected, m) {
			t.Errorf("%s: (-want +got)\n%v", test.Description, cmp.Diff(test.Expected, m))
		}
	}
}
