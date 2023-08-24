package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall/js"
	"time"
)

func main() {
	done := make(chan int, 0)
	js.Global().Set("pollingMessages", js.FuncOf(PollingMessagesJs))
	<-done
}

func PollingMessagesJs(this js.Value, args []js.Value) interface{} {
	go PollingMessages(args[0].String(), args[1].String(), args[2].Bool(), args[3].String(), args[4].String(),
		args[5], args[6])
	return nil
}

func TryPushAndPull(account string, password string, revoke bool, board string, article string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ptt := NewPttClient(ctx)
	err := ptt.Connect()
	if err != nil {
		return
	}
	defer ptt.Close()

	err = ptt.Login(os.Getenv("account"), os.Getenv("password"), false)
	if err != nil {
		if errors.Is(err, AuthError) {
			fmt.Println("密碼不對或無此帳號")
		}
		return
	}

	i := 1
	go func() {
		time.Sleep(3 * time.Second)
		for {
			err = ptt.PushMessage(strconv.Itoa(i))
			i += 1
			if err != nil {
				fmt.Println(err)
			}
			time.Sleep(1 * time.Second)
		}
	}()

	go func() {
		err = ptt.PullMessages(board, article)
		if err != nil {
			if errors.Is(err, WrongArticleIdError) {
				fmt.Println("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")
			}
			return
		}
	}()

	for {

	}
}

func PushMessage(account string, password string, board string, article string, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ptt := NewPttClient(ctx)
	err := ptt.Connect()
	if err != nil {
		return
	}
	defer ptt.Close()

	err = ptt.Login(account, password, false)
	if err != nil {
		if errors.Is(err, AuthError) {
			fmt.Println("密碼不對或無此帳號")
		}
		return
	}

	err = ptt.PushMessage(message)
	if err != nil {
		fmt.Println(err)
	}
}

func logError(msg string, e error) {
	fmt.Println(msg, e)
}

func PollingMessages(account string, password string, revokeOthers bool, board string, article string,
	callback js.Value, reject js.Value) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	ptt := NewPttClient(ctx)
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
