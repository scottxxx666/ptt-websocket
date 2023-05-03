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
	return all, nil
}

func send(conn net.Conn, data []byte) error {
	err := wsutil.WriteClientMessage(conn, ws.OpBinary, data)
	if err != nil {
		fmt.Println("send fail")
		return err
	}
	return nil
}

func init() {
	if err := godotenv.Load(); err != nil {
		panic(err)
		return
	}
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
		} else if bytes.Contains(d, []byte("請按任意鍵繼續")) {
			err = send(conn, []byte(" "))
			if err != nil {
				fmt.Println(err)
				return
			}
		} else if bytes.Contains(d, []byte("您想刪除其他重複登入的連線嗎")) {
			panic("duplicate login")
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

	searchBoard := []byte("C_Chat\r")
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

	if err = send(conn, []byte("eeeeee\rY\r")); err != nil {
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
			err := send(conn, []byte(" "))
			panic(err)
		}
	}
}
