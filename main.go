package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"time"
)

func init() {
	if err := godotenv.Load(); err != nil {
		panic(err)
		return
	}
}

func main() {
	PollingMessages(os.Getenv("account"), os.Getenv("password"), true, os.Getenv("board"), os.Getenv("article"))
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

	err = ptt.Login(account, password, revokeOthers)
	if err != nil {
		if errors.Is(err, AuthError) {
			fmt.Println("密碼不對或無此帳號")
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
