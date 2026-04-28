package main

import (
	"flag"
	"log"
)

func main() {
	// 命令行参数
	port := flag.String("port", "8080", "服务端口")
	url := flag.String("url", "ws://127.0.0.1", "WebSocket 连接地址")
	path := flag.String("path", "/ws/jserver", "WebSocket 路径")
	redisAddr := flag.String("redis", "", "Redis 地址（如 127.0.0.1:6379），为空则不启用 Redis 判题")

	flag.Parse()

	// 1. 建立 WebSocket 连接
	wsConn, localIP, err := NewWSConnection(*url, *port, *path)
	if err != nil {
		log.Fatal("WebSocket 连接失败:", err)
	}
	defer wsConn.Close()

	// 获取网卡名
	netInterface, err = GetInterfaceByIP(localIP)
	if err != nil {
		log.Printf("获取网卡名失败: %v", err)
	}
	log.Println("连接成功")

	// 2. 初始化 IO 基准值
	initIO()

	// 3. 启动系统监控采集协程
	go collectTask()

	// 4. 启动监控上报协程（通过 WebSocket）
	go sendTask(wsConn)

	// 5. 初始化 Docker 客户端
	if err := InitDocker(); err != nil {
		log.Printf("Docker 初始化失败（不影响其他功能）: %v", err)
	}

	// 6. 初始化 Redis 并启动判题循环（可选）
	if *redisAddr != "" {
		if err := InitRedis(*redisAddr, "", 0); err != nil {
			log.Fatal("Redis 连接失败:", err)
		}
		go RunJudgeLoop(wsConn)
	} else {
		log.Println("未配置 Redis，Redis 判题功能未启用")
	}

	// 7. 主协程：读取并分发服务端消息
	DispatchServerMessage(wsConn)
}
