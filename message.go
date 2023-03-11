package main

import (
	"bytes"
	"strings"
	"time"
)

type Message struct {
	EnvelopeSender string
	ReceivedField  string
	Buf            []byte
	SessionId      string
	EOL            string
}

func (be *Backend) NewMessage(s *Session, b []byte) *Message {
	c := be.Config

	var received string
	if c.C.AddReceived {
		var words []string

		words = append(words, "from")
		words = append(words, s.HelloHost)
		rh, err := addrToIP(s.RemoteAddr)
		if err == nil {
			words = append(words, "("+addressLiteral(rh)+")")
		}

		words = append(words, "by")
		words = append(words, c.S.Domain)
		lh, err := addrToIP(s.LocalAddr)
		if err == nil {
			words = append(words, "("+addressLiteral(lh)+")")
		}

		words = append(words, "with")
		var with string
		if c.S.LMTP {
			with = "LMTP"
		} else {
			// FIXME: "SMTP" is possible
			with = "ESMTP"
		}
		words = append(words, with)

		words = append(words, "id")
		words = append(words, s.Id)

		if len(s.RcptTos) == 1 {
			words = append(words, "for")
			words = append(words, "<"+s.RcptTos[0]+">")
		}

		words[len(words)-1] += ";"

		words = append(words, time.Now().Format(time.RFC1123Z))

		lines := []string{"Received:"}
		for _, w := range words {
			if 78 < len(lines[len(lines)-1]+" "+w) {
				lines = append(lines, " "+w)
			} else {
				lines[len(lines)-1] += " " + w
			}
		}
		received = strings.Join(lines, "\r\n")
	}

	return &Message{
		EnvelopeSender: s.MailFrom,
		ReceivedField:  received,
		Buf:            b,
		SessionId:      s.Id,
		EOL:            c.C.EOL,
	}
}

func (m *Message) Serialized() []byte {
	b := []byte("Return-Path: <" + m.EnvelopeSender + ">\r\n")
	if m.ReceivedField != "" {
		b = append(b, []byte(m.ReceivedField+"\r\n")...)
	}
	if len(m.Buf) != 0 {
		b = append(b, m.Buf...)
	}

	eol := m.EOL
	if eol != "" && eol != "\r\n" {
		b = bytes.Replace(b, []byte("\r\n"), []byte(eol), -1)
	}

	return b
}
