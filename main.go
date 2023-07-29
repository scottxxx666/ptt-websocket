package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/joho/godotenv"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"time"
)

// keep websocket reading until message size less than 1024
func read(conn net.Conn) ([]byte, error) {
	var all []byte
	for {
		data, _, err := wsutil.ReadServerData(conn)
		if err != nil {
			return nil, err
		}
		reader := transform.NewReader(bytes.NewBuffer(data), traditionalchinese.Big5.NewDecoder())
		big5, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		// append big5 to all
		all = append(all, big5...)
		if len(data) < 1024 {
			break
		}
	}
	return cleanData(all), nil
}

func send(conn net.Conn, data []byte) error {
	err := wsutil.WriteClientMessage(conn, ws.OpBinary, data)
	if err != nil {
		fmt.Println("send fail")
		return err
	}
	return nil
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

func init() {
	if err := godotenv.Load(); err != nil {
		panic(err)
		return
	}
}

type Message struct {
	Time    time.Time
	Message string
	User    string
}

func (m *Message) Equal(input *Message) bool {
	return m.User == input.User && m.Time.Equal(input.Time) && m.Message == input.Message
}

func (m *Message) Null() bool {
	fmt.Println(m.User)
	fmt.Println(m.User == "")
	return m.User == ""
}

func main() {
	dialer := ws.DefaultDialer
	dialer.Header = ws.HandshakeHeaderHTTP(http.Header{"Origin": []string{"https://term.ptt.cc"}})
	conn, _, _, err := dialer.Dial(context.Background(), "wss://ws.ptt.cc/bbs")
	if err != nil {
		fmt.Println("connect fail")
		fmt.Println(err)
		return
	}

	defer conn.Close()

	for {
		d, err := read(conn)
		if err != nil {
			fmt.Println("read fail")
			fmt.Println(err)
			return
		}
		fmt.Printf("%s\n", d)

		if bytes.Contains(d, []byte("請輸入代號")) {
			fmt.Println("send account")
			account := []byte(os.Getenv("account"))
			for i := range account {
				err = send(conn, account[i:i+1])
				if err != nil {
					fmt.Println(err)
					return
				}
				if _, err := read(conn); err != nil {
					fmt.Println(err)
					return
				}
			}
			err = send(conn, []byte("\r"))
			if err != nil {
				fmt.Println(err)
				return
			}
		} else if bytes.Contains(d, []byte("請輸入您的密碼")) {
			password := []byte(os.Getenv("password") + "\r")
			for i := range password {
				if err = send(conn, password[i:i+1]); err != nil {
					fmt.Println(err)
					return
				}
			}
		} else if bytes.Contains(d, []byte("密碼不對")) {
			panic("wrong password")
		} else if bytes.Contains(d, []byte("按任意鍵繼續")) {
			err = send(conn, []byte(" "))
			if err != nil {
				fmt.Println(err)
				return
			}
		} else if bytes.Contains(d, []byte("您想刪除其他重複登入的連線嗎")) {
			err = send(conn, []byte(os.Getenv("revokeOthers")+"\r"))
			if err != nil {
				fmt.Println(err)
				return
			}
		} else if bytes.Contains(d, []byte("【主功能表】")) {
			break
		}
	}

	searchBoardCmd := []byte("s")
	if err = send(conn, searchBoardCmd); err != nil {
		fmt.Println(err)
		return
	}
	d, err := read(conn)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s\n", d)

	searchBoard := []byte(os.Getenv("board"))
	for i := range searchBoard {
		if err = send(conn, searchBoard[i:i+1]); err != nil {
			fmt.Println(err)
			return
		}
		_, err := read(conn)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%s\n", d)
	}

	if err = send(conn, []byte("\r")); err != nil {
		fmt.Println(err)
		return
	}
	for {
		d, err = read(conn)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%s\n", d)
		if bytes.Contains(d, []byte("按任意鍵繼續")) {
			if err = send(conn, []byte(" ")); err != nil {
				fmt.Println(err)
				return
			}
			d, err = read(conn)
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Printf("%s\n", d)
			break
		}
	}

	articleId := []byte(os.Getenv("article") + "\r")
	for i := range articleId {
		if err = send(conn, articleId[i:i+1]); err != nil {
			fmt.Println(err)
			return
		}
		d, err = read(conn)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%s\n", d)
	}

	var lastMessage *Message
	for {
		if err = send(conn, []byte("\rG")); err != nil {
			fmt.Println(err)
			return
		}
		d, err = read(conn)
		if err != nil {
			fmt.Println(err)
			return
		}

		// parse line by line
		lines := bytes.Split(d, []byte("\n"))

		lastLineNum := len(lines) - 2
		messages := make([]Message, 0)
		for i := lastLineNum; i >= 0; i-- {
			message, err := parseMessage(lines[i])
			if err != nil {
				fmt.Printf("parse message err: %s\n", err)
				break
			}
			if lastMessage != nil && (message.Equal(lastMessage) || message.Time.Before(lastMessage.Time)) {
				break
			}
			messages = append(messages, *message)
		}

		if len(messages) > 0 {
			lastMessage = &messages[0]
		}
		for i := len(messages) - 1; i >= 0; i-- {
			message := messages[i]
			fmt.Printf("%s: %s %s\n", message.User, message.Message, message.Time)
		}

		time.Sleep(1 * time.Second)

		if err = send(conn, []byte("q")); err != nil {
			fmt.Println(err)
			return
		}
		d, err = read(conn)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	if err = send(conn, []byte("qeeeeee\rY\r")); err != nil {
		fmt.Println(err)
		return
	}
	for {
		d, err = read(conn)
		if err != nil {
			fmt.Println(err)
			return
		}

		if bytes.Contains(d, []byte("按任意鍵繼續")) {
			err := send(conn, []byte(" "))
			if err != nil {
				panic(err)
			}
		}
	}
}

func parseMessage(l []byte) (*Message, error) {
	date := l[len(l)-11:]
	t, err := time.Parse("01/02 15:04", string(date))
	if err != nil {
		fmt.Printf("parse time error %s \n", err)
		return nil, err
	}
	space := bytes.Index(l, []byte(" "))
	colon := bytes.Index(l, []byte(":"))
	id := l[space+1 : colon]
	return &Message{
		Time:    t,
		User:    string(id),
		Message: string(l[colon+2 : len(l)-11]),
	}, nil
}
