package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type HeartBeatMessage struct {
}

func main() {
	// WebSocket 服务端地址
	port := flag.String("port", "8080", "服务端口")
	url := flag.String("url", "ws://localhost", "Web Socket连接地址")
	path := flag.String("path", "/ws/jserver", "Web Socket路径")

	flag.Parse()

	fp := fmt.Sprintf("%s:%s%s", *url, *port, *path)
	log.Println(fp)
	// 1. 建立连接
	conn, _, err := websocket.DefaultDialer.Dial(fp, nil)

	if err != nil {
		log.Fatal("连接失败:", err)
	} else {
		log.Println("连接成功")
	}
	defer conn.Close()

	// 2. 启动协程异步读取服务端消息
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("读取失败:", err)
				return
			}
			log.Printf("收到服务端消息: %s", msg)
		}
	}()

	// 3. 循环发送消息
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		err := conn.WriteMessage(websocket.TextMessage, []byte("Hello WebSocket!"))
		if err != nil {
			log.Println("发送失败:", err)
			return
		}
	}
}
