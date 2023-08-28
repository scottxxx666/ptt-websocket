package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"io"
	"math"
	"nhooyr.io/websocket"
	"regexp"
	"sync"
	"syscall/js"
	"time"
)

var WrongArticleIdError = errors.New("WRONG_ARTICLE")
var AuthError = errors.New("AUTH_FAIL")
var MsgEncodeError = errors.New("MSG_ENCODE_ERR")
var NotFinishArticleError = errors.New("NOT_FINISH_ARTICLE")

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
	return m.User == ""
}

type PttClient struct {
	ctx    context.Context
	conn   *websocket.Conn
	Cancel context.CancelFunc
	lock   sync.Mutex
}

func NewPttClient(context context.Context) *PttClient {
	return &PttClient{ctx: context}
}

func (ptt *PttClient) Connect() error {
	var err error
	ptt.conn, _, err = websocket.Dial(ptt.ctx, "wss://ws.ptt.cc/bbs", nil)
	if err != nil {
		logError("connect websocket error", err)
		return err
	}
	return nil
}

func (ptt *PttClient) Close() {
	ptt.conn.Close(websocket.StatusInternalError, "")
}

func (ptt *PttClient) Login(account string, password string, revokeOthers bool) error {
	for {
		d, err := read(ptt.conn)
		if err != nil {
			logError("read fail", err)
			return err
		}

		if bytes.Contains(d, []byte("密碼不對或無此帳號")) {
			return AuthError
		} else if bytes.Contains(d, []byte("請輸入代號")) {
			accountByte := []byte(account)
			for i := range accountByte {
				err = send(ptt.conn, accountByte[i:i+1])
				if err != nil {
					logError("send account", err)
					return err
				}
				if _, err = read(ptt.conn); err != nil {
					logError("send account read", err)
					return err
				}
			}
			err = send(ptt.conn, []byte("\r"))
			if err != nil {
				logError("send account enter", err)
				return err
			}
		} else if bytes.Contains(d, []byte("請輸入您的密碼")) {
			passwordByte := []byte(password + "\r")
			for i := range passwordByte {
				if err = send(ptt.conn, passwordByte[i:i+1]); err != nil {
					logError("password send", err)
					return err
				}
			}
		} else if bytes.Contains(d, []byte("按任意鍵繼續")) {
			err = send(ptt.conn, []byte(" "))
			if err != nil {
				logError("send continue", err)
				return err
			}
		} else if bytes.Contains(d, []byte("您想刪除其他重複登入的連線嗎")) {
			revoke := "N"
			if revokeOthers {
				revoke = "Y"
			}
			err = send(ptt.conn, []byte(revoke+"\r"))
			if err != nil {
				logError("send revoke others", err)
				return err
			}
		} else if bytes.Contains(d, []byte("您要刪除以上錯誤嘗試的記錄嗎?")) {
			err = send(ptt.conn, []byte("n\r"))
			if err != nil {
				logError("delete login fails", err)
				return err
			}
		} else if bytes.Contains(d, []byte("您有一篇文章尚未完成")) {
			return NotFinishArticleError
		} else if bytes.Contains(d, []byte("【主功能表】")) {
			break
		}
	}
	return nil
}

func (ptt *PttClient) PullMessages(board string, article string, callback js.Value) error {
	var lastMessage *Message
	var msgId int32 = 1
	for {
		ptt.lock.Lock()
		err := ptt.EnterBoard(board)
		if err != nil {
			return err
		}

		page, err := ptt.EnterArticle(article)
		if err != nil {
			return err
		}
		page, err = ptt.pageEnd(page)
		if err != nil {
			return err
		}

		fmt.Printf("screen: %s\n", page)
		var messages []Message
		messages, msgId = ptt.parsePageMessages(page, msgId, lastMessage)
		ptt.lock.Unlock()
		if len(messages) > 0 {
			lastMessage = &messages[len(messages)-1]
		}
		json, err := json.Marshal(messages)
		if err != nil {
			logError("marshal json failed", err)
			return err
		}
		callback.Invoke(string(json))

		time.Sleep(1 * time.Second)
	}
}

func (ptt *PttClient) parsePageMessages(page []byte, msgId int32, lastMessage *Message) ([]Message, int32) {
	lines := bytes.Split(page, []byte("\n"))

	lastLineNum := len(lines) - 2
	reversedMsgs := make([]Message, 0)
	for i := lastLineNum; i >= 0; i-- {
		if bytes.Contains(lines[i], []byte("※ 文章網址:")) || bytes.Contains(lines[i], []byte("※ 發信站:")) {
			break
		}
		message, err := parseMessage(lines[i], msgId)
		msgId = (msgId + 1) % math.MaxInt32
		if err != nil {
			logError("parse message error", err)
			break
		}
		if lastMessage != nil && (message.Equal(lastMessage) || message.Time.Before(lastMessage.Time)) {
			break
		}
		reversedMsgs = append(reversedMsgs, *message)
	}

	msgs := make([]Message, 0, len(reversedMsgs))
	msgTime := time.Now()
	for i := len(reversedMsgs) - 1; i >= 0; i-- {
		msg := reversedMsgs[i]

		// assign last msg time if parse current msg time failed
		// since we only use last message's time, so assign time from prev is better
		if msg.Time.IsZero() {
			msg.Time = msgTime
		} else {
			msgTime = msg.Time
		}
		msgs = append(msgs, msg)
	}

	return msgs, msgId
}

func (ptt *PttClient) pageEnd(page []byte) ([]byte, error) {
	if bytes.Contains(page, []byte("頁 (100%)  目前顯示")) {
		return page, nil
	}
	err := send(ptt.conn, []byte("G"))
	if err != nil {
		logError("send article bottom command", err)
		return nil, err
	}
	end, err := read(ptt.conn)
	if err != nil {
		logError("read article bottom", err)
		return nil, err
	}
	return end, nil
}

func (ptt *PttClient) PushMessage(message string) error {
	encoder := traditionalchinese.Big5.NewEncoder()
	msgBytes, _, err := transform.Bytes(encoder, []byte(message+"\r"))
	if err != nil {
		logError("encode big5 error", err)
		return MsgEncodeError
	}

	ptt.lock.Lock()
	defer ptt.lock.Unlock()
	if err := send(ptt.conn, []byte("X")); err != nil {
		logError("send push command", err)
		return err
	}
	d, err := read(ptt.conn)
	if err != nil {
		logError("read push command", err)
		return err
	}

	if bytes.Contains(d, []byte("給它噓聲")) {
		if err = send(ptt.conn, []byte("1")); err != nil {
			logError("send push command type", err)
			return err
		}
		d, err = read(ptt.conn)
		if err != nil {
			logError("read push command", err)
			return err
		}
	}

	if err = send(ptt.conn, msgBytes); err != nil {
		logError("send push command type", err)
		return err
	}
	d, err = read(ptt.conn)
	if err != nil {
		logError("read push command", err)
		return err
	}

	if err = send(ptt.conn, []byte("Y\r")); err != nil {
		logError("send push command type", err)
		return err
	}
	d, err = read(ptt.conn)
	if err != nil {
		logError("read push command", err)
		return err
	}

	return nil
}

func (ptt *PttClient) EnterArticle(article string) (firstPage []byte, err error) {
	var data []byte
	articleId := []byte(article + "\r")
	for i := range articleId {
		if err = send(ptt.conn, articleId[i:i+1]); err != nil {
			logError("send search article", err)
			return nil, err
		}
		data, err = read(ptt.conn)
		if err != nil {
			logError("read search article", err)
			return nil, err
		}
	}
	if bytes.Contains(data, []byte("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")) {
		return nil, WrongArticleIdError
	}

	if err = send(ptt.conn, []byte("\r")); err != nil {
		logError("send article enter command", err)
		return nil, err
	}
	firstPage, err = read(ptt.conn)
	if err != nil {
		logError("read article bottom", err)
		return nil, err
	}
	return firstPage, nil
}

func (ptt *PttClient) EnterBoard(board string) error {
	searchBoardCmd := []byte("s")
	err := send(ptt.conn, searchBoardCmd)
	if err != nil {
		logError("send search board command", err)
		return err
	}
	d, err := read(ptt.conn)
	if err != nil {
		logError("read search board command", err)
		return err
	}

	searchBoard := []byte(board)
	for i := range searchBoard {
		if err = send(ptt.conn, searchBoard[i:i+1]); err != nil {
			logError("send search board name", err)
			return err
		}
		_, err = read(ptt.conn)
		if err != nil {
			logError("read search board name", err)
			return err
		}
	}

	if err = send(ptt.conn, []byte("\r")); err != nil {
		logError("send enter after search board", err)
		return err
	}
	for {
		d, err = read(ptt.conn)
		if err != nil {
			logError("read after enter board", err)
			return err
		}
		if bytes.Contains(d, []byte("【板主:")) && bytes.Contains(d, []byte("看板《")) &&
			!bytes.Contains(d, []byte("按任意鍵繼續")) && !bytes.Contains(d, []byte("動畫播放中... 可按 q, Ctrl-C 或其它任意鍵停止")) {
			break
		}
		if err = send(ptt.conn, []byte(" ")); err != nil {
			logError("send after enter board", err)
			return err
		}
	}
	return nil
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
	var t time.Time
	var err error
	if len(l) < 11 || (!bytes.Equal(l[0:4], []byte("推 ")) && !bytes.Equal(l[0:4], []byte("噓 ")) && !bytes.Equal(l[0:4], []byte("→ "))) {
		fmt.Printf("not message line: %s\n", l)
		return nil, errors.New("not message line")
	}
	date := l[len(l)-11:]
	t, err = time.Parse("01/02 15:04", string(date))
	if err != nil {
		fmt.Printf("parse time error %s \n", err)
		fmt.Printf("error line: %s\n", l)
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
		logError("send fail", err)
		return err
	}
	return nil
}
