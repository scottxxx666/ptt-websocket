package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"io"
	"math"
	"net/http"
	"nhooyr.io/websocket"
	"os"
	"regexp"
	"time"
)

func init() {
	if err := godotenv.Load(); err != nil {
		panic(err)
		return
	}
}

type Message struct {
	Id      int32     `json:"id"`
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	User    string    `json:"user"`
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
	PollingMessages(os.Getenv("account"), os.Getenv("password"), true, os.Getenv("board"), os.Getenv("article"))
}

func logError(e error, msg string) {
	fmt.Println(msg, e)
}

func PollingMessages(account string, password string, revokeOthers bool, board string, article string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// conn, _, err := websocket.Dial(ctx, "wss://ws.ptt.cc/bbs", nil)
	conn, _, err := websocket.Dial(ctx, "wss://ws.ptt.cc/bbs", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://term.ptt.cc"}}})
	if err != nil {
		logError(err, "connect websocket error")
		return
	}
	defer conn.Close(websocket.StatusInternalError, "the sky is falling")

	var msgId int32 = 1
	for {
		d, err := read(conn)
		if err != nil {
			fmt.Println("read fail")
			fmt.Println(err)
			return
		}
		fmt.Printf("%s\n", d)

		if bytes.Contains(d, []byte("密碼不對或無此帳號")) {
			panic("wrong password")
		} else if bytes.Contains(d, []byte("請輸入代號")) {
			fmt.Println("send account")
			accountByte := []byte(account)
			for i := range accountByte {
				err = send(conn, accountByte[i:i+1])
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
			passwordByte := []byte(password + "\r")
			for i := range passwordByte {
				if err = send(conn, passwordByte[i:i+1]); err != nil {
					fmt.Println(err)
					return
				}
			}
		} else if bytes.Contains(d, []byte("按任意鍵繼續")) {
			err = send(conn, []byte(" "))
			if err != nil {
				fmt.Println(err)
				return
			}
		} else if bytes.Contains(d, []byte("您想刪除其他重複登入的連線嗎")) {
			revoke := "N"
			if revokeOthers {
				revoke = "Y"
			}
			err = send(conn, []byte(revoke+"\r"))
			if err != nil {
				fmt.Println(err)
				return
			}
		} else if bytes.Contains(d, []byte("您要刪除以上錯誤嘗試的記錄嗎?")) {
			err = send(conn, []byte("n\r"))
			if err != nil {
				fmt.Println(err)
				return
			}
		} else if bytes.Contains(d, []byte("【主功能表】")) {
			break
		}
	}

	var lastMessage *Message
	for {
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

		searchBoard := []byte(board)
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
			if bytes.Contains(d, []byte("【板主:")) && bytes.Contains(d, []byte("看板《")) &&
				!bytes.Contains(d, []byte("按任意鍵繼續")) && !bytes.Contains(d, []byte("動畫播放中... 可按 q, Ctrl-C 或其它任意鍵停止")) {
				break
			}
			if err = send(conn, []byte(" ")); err != nil {
				fmt.Println(err)
				return
			}
		}

		articleId := []byte(article + "\r")
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
		if bytes.Contains(d, []byte("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")) {
			panic("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")
		}

		if err = send(conn, []byte("\rG")); err != nil {
			fmt.Println(err)
			return
		}
		d, err = read(conn)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("screen: %s\n", d)
		// parse line by line
		lines := bytes.Split(d, []byte("\n"))

		lastLineNum := len(lines) - 2
		messages := make([]Message, 0)
		for i := lastLineNum; i >= 0; i-- {
			message, err := parseMessage(lines[i], msgId)
			msgId = (msgId + 1) % math.MaxInt32
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
	}

	conn.Close(websocket.StatusNormalClosure, "")
}

// keep websocket reading until message size less than 1024
func read(conn *websocket.Conn) ([]byte, error) {
	var all []byte
	for {
		_, data, err := conn.Read(context.Background())
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

func parseMessage(l []byte, i int32) (*Message, error) {
	fmt.Printf("line: %s\n", l)
	date := l[len(l)-11:]
	t, err := time.Parse("01/02 15:04", string(date))
	if err != nil {
		fmt.Printf("parse time error %s \n", err)
		t = time.Now()
	}
	space := bytes.Index(l, []byte(" "))
	colon := bytes.Index(l, []byte(":"))
	user := l[space+1 : colon]
	return &Message{
		Id:      i,
		Time:    t,
		User:    string(bytes.TrimRight(user, " ")),
		Message: string(bytes.TrimRight(l[colon+2:len(l)-11], " ")),
	}, nil
}

func send(c *websocket.Conn, data []byte) error {
	err := c.Write(context.Background(), websocket.MessageBinary, data)
	if err != nil {
		logError(err, "send fail")
		return err
	}
	return nil
}
