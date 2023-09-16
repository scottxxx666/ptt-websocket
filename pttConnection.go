package main

import (
	"bytes"
	"context"
	"golang.org/x/text/transform"
	"io"
	"net/http"
	"nhooyr.io/websocket"
	"regexp"
)

type PttConnection struct {
	ctx  context.Context
	conn *websocket.Conn
}

func NewPttConnection(ctx context.Context) *PttConnection {
	return &PttConnection{ctx: ctx}
}

func (p *PttConnection) Connect() (err error) {
	p.conn, _, err = websocket.Dial(p.ctx, "wss://ws.ptt.cc/bbs", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://term.ptt.cc"}}})
	if err != nil {
		return err
	}
	return nil
}

func (p *PttConnection) Close() {
	p.conn.Close(websocket.StatusInternalError, "")
}

// keep websocket reading until message size less than 1024
func (p *PttConnection) Read() ([]byte, error) {
	var all []byte
	for {
		_, data, err := p.conn.Read(context.Background())
		if err != nil {
			return nil, err
		}
		all = append(all, data...)
		if len(data) < 1024 {
			break
		}
	}
	reader := transform.NewReader(bytes.NewBuffer(cleanData(all)), NewUaoDecoder())

	utf8, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return utf8, nil
}

func cleanData(data []byte) []byte {
	// Replace ANSI escape sequences with =ESC=.
	data = regexp.MustCompile(`\x1B`).ReplaceAll(data, nil)

	// Remove any remaining ANSI escape codes.
	data = regexp.MustCompile(`\[[\d+;]*m`).ReplaceAll(data, nil)

	// Remove carriage returns.
	data = bytes.ReplaceAll(data, []byte{'\r'}, nil)

	// Remove backspaces.
	for bytes.Contains(data, []byte{' ', '\x08'}) {
		data = bytes.ReplaceAll(data, []byte{' ', '\x08'}, nil)
	}

	// remove [H [K
	data = bytes.ReplaceAll(data, []byte("[K"), nil)
	data = bytes.ReplaceAll(data, []byte("[H"), nil)

	return data
}

func (p *PttConnection) Send(data []byte) error {
	err := p.conn.Write(context.Background(), websocket.MessageBinary, data)
	if err != nil {
		logError("send fail", err)
		return err
	}
	return nil
}
