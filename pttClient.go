package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"syscall/js"
	"time"
)

var WrongArticleIdError = errors.New("WRONG_ARTICLE")
var AuthError = errors.New("AUTH_FAIL")
var MsgEncodeError = errors.New("MSG_ENCODE_ERR")
var NotFinishArticleError = errors.New("NOT_FINISH_ARTICLE")
var PttOverloadError = errors.New("PTT_OVERLOAD")

type Message struct {
	Id      int32     `json:"id"`
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	User    string    `json:"user"`
}

func (m *Message) Equal(input *Message) bool {
	return m.User == input.User && m.Message == input.Message
}

func (m *Message) Null() bool {
	return m.User == ""
}

type PttClient struct {
	ctx          context.Context
	conn         *PttConnection
	Cancel       context.CancelFunc
	lock         sync.Mutex
	Screen       []byte
	Debug        bool
	timeout      time.Duration
	loginTimeout time.Duration
}

func NewPttClient(context context.Context) *PttClient {
	return &PttClient{
		ctx:          context,
		conn:         NewPttConnection(context),
		Debug:        false,
		timeout:      2000 * time.Millisecond,
		loginTimeout: 30000 * time.Millisecond,
	}
}

func (ptt *PttClient) Connect() (err error) {
	err = ptt.conn.Connect()
	if err != nil {
		logError("connect error", err)
		return err
	}
	return nil
}

func (ptt *PttClient) Close() {
	ptt.conn.Close()
}

func (ptt *PttClient) Login(account string, password string, revokeOthers bool) (err error) {
	for {
		err = ptt.Read(ptt.loginTimeout)
		ptt.logDebug("Login----\n%s\n----\n", ptt.Screen)
		if err != nil {
			logError("read fail", err)
			return err
		}

		if bytes.Contains(ptt.Screen, []byte("系統過載, 請稍後再來")) {
			return PttOverloadError
		} else if bytes.Contains(ptt.Screen, []byte("密碼不對或無此帳號")) {
			return AuthError
		} else if bytes.Contains(ptt.Screen, []byte("請輸入代號")) {
			accountByte := []byte(account)
			for i := range accountByte {
				err = ptt.conn.Send(accountByte[i : i+1])
				if err != nil {
					logError("send account", err)
					return err
				}
				err = ptt.Read(6000 * time.Millisecond)
				if err != nil {
					logError("send account read", err)
					return err
				}
			}
			err = ptt.conn.Send([]byte("\r"))
			if err != nil {
				logError("send account enter", err)
				return err
			}
		} else if bytes.Contains(ptt.Screen, []byte("請輸入您的密碼")) {
			passwordByte := []byte(password + "\r")
			for i := range passwordByte {
				if err = ptt.conn.Send(passwordByte[i : i+1]); err != nil {
					logError("password send", err)
					return err
				}
			}
		} else if bytes.Contains(ptt.Screen, []byte("按任意鍵繼續")) {
			err = ptt.conn.Send([]byte(" "))
			if err != nil {
				logError("send continue", err)
				return err
			}
		} else if bytes.Contains(ptt.Screen, []byte("您想刪除其他重複登入的連線嗎")) {
			revoke := "N"
			if revokeOthers {
				revoke = "Y"
			}
			err = ptt.conn.Send([]byte(revoke + "\r"))
			if err != nil {
				logError("send revoke others", err)
				return err
			}
		} else if bytes.Contains(ptt.Screen, []byte("您要刪除以上錯誤嘗試的記錄嗎?")) {
			err = ptt.conn.Send([]byte("n\r"))
			if err != nil {
				logError("delete login fails", err)
				return err
			}
		} else if bytes.Contains(ptt.Screen, []byte("您有一篇文章尚未完成")) {
			return NotFinishArticleError
		} else if bytes.Contains(ptt.Screen, []byte("您保存信件數目")) || bytes.Contains(ptt.Screen, []byte("郵件選單")) {
			// 您保存信件數目...超出上限 200, 請整理
			// need send and read twice
			err = ptt.conn.Send([]byte("q"))
			if err != nil {
				logError("login mail fails", err)
				return err
			}
		} else if bytes.Contains(ptt.Screen, []byte("主功能表")) {
			ptt.logDebug("login success\n")
			break
		}
	}
	return nil
}

func (ptt *PttClient) Read(duration time.Duration) error {
	var err error
	ptt.Screen, err = ptt.conn.Read(duration)
	return err
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

		// since sometimes get article first page not page end
		// temporarily change enterArticle to fetchArticleEnd
		err = ptt.fetchArticleEnd(article)
		if err != nil {
			return err
		}
		//
		// page, err = ptt.pageEnd(page)
		// if err != nil {
		// 	return err
		// }

		ptt.logDebug("pull message:\n%s\n", ptt.Screen)
		var messages []Message
		messages, msgId = ptt.parsePageMessages(msgId, lastMessage)
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

		time.Sleep(500 * time.Millisecond)
	}
}

func (ptt *PttClient) parsePageMessages(msgId int32, lastMessage *Message) ([]Message, int32) {
	lines := bytes.Split(ptt.Screen, []byte("\n"))

	lastLineNum := len(lines) - 2
	reversedMsgs := make([]Message, 0)
	for i := lastLineNum; i >= 0; i-- {
		if bytes.Contains(lines[i], []byte("※ 文章網址:")) || bytes.Contains(lines[i], []byte("※ 發信站:")) {
			break
		}
		message, err := parseMessage(lines[i], msgId)
		msgId = (msgId + 1) % math.MaxInt32
		if err != nil {
			continue
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

func (ptt *PttClient) pageEnd() error {
	if bytes.Contains(ptt.Screen, []byte("頁 (100%)  目前顯示")) {
		return nil
	}
	// WORKAROUND: send page end twice to force page end
	err := ptt.conn.Send([]byte("GG"))
	if err != nil {
		logError("send article bottom command", err)
		return err
	}
	err = ptt.Read(ptt.timeout)
	if err != nil {
		logError("read article bottom", err)
		return err
	}
	return nil
}

func (ptt *PttClient) PushMessage(message string) error {
	big5, err := Utf8ToUaoBig5(message)
	if err != nil {
		logError("encode big5 error", err)
		return MsgEncodeError
	}

	ptt.lock.Lock()
	defer ptt.lock.Unlock()
	if err := ptt.conn.Send([]byte("X")); err != nil {
		logError("send push command", err)
		return err
	}
	err = ptt.Read(ptt.timeout)
	if err != nil {
		logError("read push command", err)
		return err
	}

	if bytes.Contains(ptt.Screen, []byte("給它噓聲")) {
		if err = ptt.conn.Send([]byte("1")); err != nil {
			logError("send push command type", err)
			return err
		}
		err = ptt.Read(ptt.timeout)
		if err != nil {
			logError("read push command", err)
			return err
		}
	}

	if err = ptt.conn.Send([]byte(big5 + "\r")); err != nil {
		logError("send push command type", err)
		return err
	}
	err = ptt.Read(ptt.timeout)
	if err != nil {
		logError("read push command", err)
		return err
	}

	if err = ptt.conn.Send([]byte("Y\r")); err != nil {
		logError("send push command type", err)
		return err
	}
	err = ptt.Read(ptt.timeout)
	if err != nil {
		logError("read push command", err)
		return err
	}

	return nil
}

func (ptt *PttClient) fetchArticleEnd(article string) (err error) {
	articleId := []byte(article + "\r")
	for i := range articleId {
		if err = ptt.conn.Send(articleId[i : i+1]); err != nil {
			logError("send search article", err)
			return err
		}
		err = ptt.Read(ptt.timeout)
		if err != nil {
			logError("read search article", err)
			return err
		}
	}
	if bytes.Contains(ptt.Screen, []byte("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")) {
		return WrongArticleIdError
	}

	if err = ptt.conn.Send([]byte("\rG")); err != nil {
		logError("send article enter command", err)
		return err
	}
	err = ptt.Read(ptt.timeout)
	if err != nil {
		logError("read article bottom", err)
		return err
	}
	return nil
}

func (ptt *PttClient) EnterBoard(board string) (err error) {
	searchBoardCmd := []byte("s")
	err = ptt.conn.Send(searchBoardCmd)
	if err != nil {
		logError("send search board command", err)
		return err
	}
	err = ptt.Read(ptt.timeout)
	if err != nil {
		logError("read search board command", err)
		return err
	}

	searchBoard := []byte(board)
	for i := range searchBoard {
		if err = ptt.conn.Send(searchBoard[i : i+1]); err != nil {
			logError("send search board name", err)
			return err
		}
		err = ptt.Read(ptt.timeout)
		if err != nil {
			logError("read search board name", err)
			return err
		}
	}

	if err = ptt.conn.Send([]byte("\r")); err != nil {
		logError("send enter after search board", err)
		return err
	}
	for {
		err = ptt.Read(ptt.timeout)
		ptt.logDebug("read after enter board-\n%s\n", ptt.Screen)
		if err != nil {
			logError("read after enter board", err)
			return err
		}
		if bytes.Contains(ptt.Screen, []byte("【板主:")) && bytes.Contains(ptt.Screen, []byte("看板《")) &&
			!bytes.Contains(ptt.Screen, []byte("按任意鍵繼續")) && !bytes.Contains(ptt.Screen, []byte("動畫播放中... 可按 q, Ctrl-C 或其它任意鍵停止")) {
			break
		}
		if err = ptt.conn.Send([]byte(" ")); err != nil {
			logError("send after enter board", err)
			return err
		}
	}
	return nil
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
	if colon+2 > len(l)-11 {
		fmt.Printf("not message line: %s\n", l)
		return nil, errors.New("not message line")
	}

	return &Message{
		Id:      i,
		Time:    t,
		User:    string(bytes.TrimRight(user, " ")),
		Message: string(bytes.TrimRight(l[colon+2:len(l)-11], " ")),
	}, nil
}

func (ptt *PttClient) logDebug(format string, a ...interface{}) {
	if ptt.Debug {
		fmt.Printf(format, a...)
	}
}
