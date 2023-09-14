package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"time"
)

func init() {
	if err := godotenv.Load(); err != nil {
		panic(err)
		return
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from ", r)
		}
	}()

	PollingMessages(os.Getenv("account"), os.Getenv("password"), false, os.Getenv("board"), os.Getenv("article"))
	// PushMessage(os.Getenv("account"), os.Getenv("password"), os.Getenv("board"), os.Getenv("article"), "你好ㄚ1c!@#$%^&*()")
	// TryPushAndPull(os.Getenv("account"), os.Getenv("password"), false, os.Getenv("board"), os.Getenv("article"))
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

func PollingMessages(account string, password string, revokeOthers bool, board string, article string) {
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
			return
		} else if errors.Is(err, NotFinishArticleError) {
			fmt.Println("有文章尚未完成，請先登入後暫存或捨棄再使用 PTT Chat")
			return
		} else if errors.Is(err, PttOverloadError) {
			fmt.Println("系統過載, 請稍後再來")
			return
		}
		return
	}

	err = ptt.PullMessages(board, article)
	if err != nil {
		if errors.Is(err, WrongArticleIdError) {
			fmt.Println("找不到這個文章代碼(AID)，可能是文章已消失，或是你找錯看板了")
		}
		return
	}
}
