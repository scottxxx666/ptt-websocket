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
		args[5], args[6])
	return nil
}

func PushMessage(message string) string {
	if ptt == nil {
		panic("not connect ptt yet")
	}
	err := ptt.PushMessage(message)
	if err != nil {
		fmt.Println(err)
		return "ERRR"
	}
	return ""
}

func PushMessagesJs(this js.Value, args []js.Value) interface{} {
	var res string
	go func() {
		res = PushMessage(args[0].String())
	}()
	return res
}

func logError(msg string, e error) {
	fmt.Println(msg, e)
}

var ptt *PttClient

func PollingMessages(account string, password string, revokeOthers bool, board string, article string,
	callback js.Value, reject js.Value) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ptt = NewPttClient(ctx)
	err := ptt.Connect()
	if err != nil {
		return
	}
	defer ptt.Close()

	err = ptt.Login(account, password, false)
	if err != nil {
		if errors.Is(err, AuthError) {
			reject.Invoke("密碼不對或無此帳號")
		}
		return
	}

	err = ptt.PullMessages(board, article, callback)
	if err != nil {
		if errors.Is(err, WrongArticleIdError) {
			reject.Invoke("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")
		}
		return
	}
}
