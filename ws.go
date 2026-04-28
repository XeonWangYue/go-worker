package main

import (
	"encoding/json"
	"log"
	"net"
	"sync"

	"github.com/gorilla/websocket"
)

// WSConnection WebSocket 连接封装
type WSConnection struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// NewWSConnection 创建 WebSocket 连接
func NewWSConnection(url, port, path string) (*WSConnection, string, error) {
	fullURL := url + ":" + port + path
	log.Println("连接地址:", fullURL)

	conn, _, err := websocket.DefaultDialer.Dial(fullURL, nil)
	if err != nil {
		return nil, "", err
	}

	localAddr, ok := conn.NetConn().LocalAddr().(*net.TCPAddr)
	if !ok {
		conn.Close()
		return nil, "", err
	}
	localIP := localAddr.IP.String()
	log.Println("本地IP:", localIP)

	wsConn := &WSConnection{conn: conn}
	return wsConn, localIP, nil
}

// Send 发送 JSON 消息（线程安全）
func (w *WSConnection) Send(msg WorkerMsg) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(msg)
}

// ReadMessage 读取消息
func (w *WSConnection) ReadMessage() (int, []byte, error) {
	return w.conn.ReadMessage()
}

// Close 关闭连接
func (w *WSConnection) Close() error {
	return w.conn.Close()
}

// DispatchServerMessage 读取并分发服务端消息
func DispatchServerMessage(wsConn *WSConnection) {
	for {
		_, msg, err := wsConn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败:", err)
			return
		}
		log.Printf("收到服务端消息: %s", msg)

		var serverMsg ServerMsg
		if err := json.Unmarshal(msg, &serverMsg); err != nil {
			log.Printf("解析消息失败: %v", err)
			continue
		}

		switch serverMsg.MsgType {
		case "create":
			handleCreate(wsConn, serverMsg.Data)
		case "kill":
			handleKill(wsConn, serverMsg.Data)
		case "judge":
			// 通过 WebSocket 下发的判题请求（从 Redis 拉取任务）
			handleJudgeRequest(wsConn, serverMsg.Data)
		case "docker_container_list", "docker_container_start", "docker_container_stop",
			"docker_container_remove", "docker_container_create", "docker_container_exec",
			"docker_image_list", "docker_image_pull", "docker_image_remove":
			HandleDockerCommand(wsConn, serverMsg.MsgType, serverMsg.Data)
		default:
			// 兼容旧协议：直接当代码执行
			log.Println("未知命令类型，尝试作为代码执行:", serverMsg.MsgType)
			config := ExecConfig{
				Code:      string(msg),
				Language:  "python",
				TimeLimit: 1000,
				MemLimit:  32,
				Input:     "1 2",
			}
			go ExecCode(config)
		}
	}
}

// handleCreate 处理创建子进程命令
func handleCreate(wsConn *WSConnection, data json.RawMessage) {
	var cmd CreateCmdData
	if err := json.Unmarshal(data, &cmd); err != nil {
		log.Printf("解析 create 命令失败: %v", err)
		return
	}
	log.Printf("创建子进程: %s", cmd.Command)
	// TODO: 实际创建子进程逻辑
}

// handleKill 处理终止子进程命令
func handleKill(wsConn *WSConnection, data json.RawMessage) {
	var cmd KillCmdData
	if err := json.Unmarshal(data, &cmd); err != nil {
		log.Printf("解析 kill 命令失败: %v", err)
		return
	}
	log.Printf("终止子进程 PID: %d", cmd.Pid)
	// TODO: 实际终止子进程逻辑
}

// handleJudgeRequest 处理判题请求
func handleJudgeRequest(wsConn *WSConnection, data json.RawMessage) {
	var req JudgeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Printf("解析判题请求失败: %v", err)
		return
	}
	log.Printf("收到判题请求，任务ID: %s", req.TaskId)
	// 通知 Redis 判题循环（已由 Redis 循环自动处理）
	// 这里可以用于触发即时判题或特殊处理
}
