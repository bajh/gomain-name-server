package main

import (
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
				Questions: []Question{
					Question{
						Name: [][]byte{
							[]byte("google"),
							[]byte("com"),
						},
						Type:  TypeNS,
						Class: ClassIN,
					},
				},
			},
			Expected: []byte{
				// Header
				1, 44, 143, 133, 1, 1, 0, 2, 255, 255, 2, 0,
				// Questions
				// Question 1
				6, byte('g'), byte('o'), byte('o'), byte('g'), byte('l'), byte('e'),
				3, byte('c'), byte('o'), byte('m'),
				// NS
				0, 2,
				// IN
				0, 1,
			},
		},
	}
	for _, test := range tests {
		buf := test.Message.Marshal()
		if len(test.Expected) != len(buf) {
			t.Errorf("%s mismatch: expected %v, got %v", test.Description, test.Expected, buf)
			continue
		}
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
			Bytes: []byte{
				1, 44, 143, 133, 0, 1, 0, 2, 255, 255, 2, 0,
				// Questions
				// Question 1
				6, byte('g'), byte('o'), byte('o'), byte('g'), byte('l'), byte('e'),
				3, byte('c'), byte('o'), byte('m'),
				// A bug happened here! I forgot to add the null-terminator and so my Type
				// was a weird number
				0,
				// NS
				0, 2,
				// IN
				0, 1,
			},
			Expected: Message{
				ID:                  300,
				IsResponse:          true,
				OpCode:              OpCodeInverse,
				AuthoritativeAnswer: true,
				Truncated:           true,
				RecursionDesired:    true,
				RecursionAvailable:  true,
				ResponseCode:        ResponseCodeRefused,
				QdCount:             1,
				AnCount:             2,
				NSCount:             65535,
				ARCount:             512,
				Questions: []Question{
					{
						Name:  [][]byte{[]byte("google"), []byte("com")},
						Type:  TypeNS,
						Class: ClassIN,
					},
				},
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
