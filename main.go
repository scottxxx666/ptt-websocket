package main

import (
	"context"
	"errors"
	"fmt"
	"syscall/js"
	"time"
)

func main() {
	done := make(chan int, 0)
	js.Global().Set("pollingMessages", js.FuncOf(PollingMessagesJs))
	js.Global().Set("pushMessage", js.FuncOf(PushMessagesJs))
	<-done
}

func PollingMessagesJs(this js.Value, args []js.Value) interface{} {
	go PollingMessages(args[0].String(), args[1].String(), args[2].Bool(), args[3].String(), args[4].String(),
		args[5], args[6], args[7].Bool())
	return nil
}

func PushMessagesJs(this js.Value, args []js.Value) interface{} {
	go PushMessage(args[0].String(), args[1])
	return nil
}

func PushMessage(message string, reject js.Value) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
			reject.Invoke(r)
		}
	}()

	if ptt == nil {
		reject.Invoke("尚未登入 PTT")
	}
	err := ptt.PushMessage(message)
	if err != nil {
		fmt.Println(err)
		if errors.Is(err, MsgEncodeError) {
			reject.Invoke(MsgEncodeError.Error())
			return
		}
		reject.Invoke("推文失敗")
	}
}

func logError(msg string, e error) {
	fmt.Println(msg, e)
}

var ptt *PttClient

func PollingMessages(account string, password string, revokeOthers bool, board string, article string,
	callback js.Value, reject js.Value, debug bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
			reject.Invoke(r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ptt = NewPttClient(ctx)
	ptt.Debug = debug
	err := ptt.Connect()
	if err != nil {
		reject.Invoke("連線 PTT 失敗")
		return
	}
	defer ptt.Close()

	err = ptt.Login(account, password, revokeOthers)
	if err != nil {
		if errors.Is(err, AuthError) {
			reject.Invoke("密碼不對或無此帳號")
			return
		} else if errors.Is(err, NotFinishArticleError) {
			reject.Invoke("有文章尚未完成，請先登入後暫存或捨棄再使用 PTT Chat")
		} else if errors.Is(err, PttOverloadError) {
			reject.Invoke("系統過載, 請稍後再來")
			return
		}
		reject.Invoke("登入失敗")
		return
	}

	err = ptt.PullMessages(board, article, callback)
	if err != nil {
		if errors.Is(err, WrongArticleIdError) {
			reject.Invoke("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")
			return
		} else if errors.Is(err, context.DeadlineExceeded) {
			reject.Invoke("DEADLINE_EXCEEDED")
			return
		}
		reject.Invoke("發生非預期的錯誤，請重試並確認資料填入正確")
		return
	}
}
