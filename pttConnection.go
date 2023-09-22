package main

import (
	"bytes"
	"context"
	"golang.org/x/text/transform"
	"io"
	"nhooyr.io/websocket"
	"regexp"
	"time"
)

type PttConnection struct {
	ctx  context.Context
	conn *websocket.Conn
}

func NewPttConnection(ctx context.Context) *PttConnection {
	return &PttConnection{ctx: ctx}
}

func (p *PttConnection) Connect() (err error) {
	p.conn, _, err = websocket.Dial(p.ctx, "wss://ws.ptt.cc/bbs", &websocket.DialOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (p *PttConnection) Close() {
	p.conn.Close(websocket.StatusInternalError, "")
}

func (p *PttConnection) readWithTimeout(duration time.Duration) ([]byte, error) {
	timeout, cancelFunc := context.WithTimeout(context.Background(), duration)
	defer cancelFunc()
	_, data, err := p.conn.Read(timeout)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// keep websocket reading until message size less than 1024
func (p *PttConnection) Read(duration time.Duration) ([]byte, error) {
	var all []byte
	for {
		data, err := p.readWithTimeout(duration)
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

	// Replace any [21;2H, [1;3H to change line
	data = regexp.MustCompile(`\[\d+;[234]H`).ReplaceAll(data, []byte("\n"))
	data = regexp.MustCompile(`\[[\d;]*H`).ReplaceAll(data, nil)

	// Remove carriage returns.
	data = bytes.ReplaceAll(data, []byte{'\r'}, nil)

	// Remove backspaces.
	data = bytes.ReplaceAll(data, []byte{' ', '\x08'}, nil)

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
