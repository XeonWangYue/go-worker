package main

import (
	"encoding/json"
	"time"
)

// ==================== WebSocket 消息类型 ====================

// ServerMsg 服务端发来的消息
type ServerMsg struct {
	MsgType string          `json:"msgType"`
	Data    json.RawMessage `json:"data"`
}

// WorkerMsg Worker 发给服务端的消息
type WorkerMsg struct {
	MsgType string      `json:"msgType"`
	Data    interface{} `json:"data"`
}

// ==================== 进程管理相关 ====================

// CreateCmdData 创建子进程的命令数据
type CreateCmdData struct {
	Command string `json:"command"`
}

// KillCmdData 终止子进程的命令数据
type KillCmdData struct {
	Pid int `json:"pid"`
}

// ProcessStat 子进程状态
type ProcessStat struct {
	Pid     int     `json:"pid"`
	CpuUsed float64 `json:"cpuUsed"`
	MemUsed float64 `json:"memUsed"`
}

// ReportMsg 统一上报消息：主机 + 所有子进程
type ReportMsg struct {
	Time           string        `json:"time"`
	HostCPU        []float64     `json:"hostCpu"`
	HostMem        float64       `json:"hostMem"`
	NetSend        uint64        `json:"netSend"`
	NetRecv        uint64        `json:"netRecv"`
	DiskRead       uint64        `json:"diskRead"`
	DiskWrite      uint64        `json:"diskWrite"`
	NetSendStr     string        `json:"netSendStr"`
	NetRecvStr     string        `json:"netRecvStr"`
	DiskReadStr    string        `json:"diskReadStr"`
	DiskWriteStr   string        `json:"diskWriteStr"`
	TotalProcesses int           `json:"totalProcesses"`
	TotalThreads   int           `json:"totalThreads"`
	Processes      []ProcessStat `json:"processes"`
}

// ==================== 判题相关 ====================

// ExecConfig 执行配置
type ExecConfig struct {
	Code      string        `json:"code"`
	Language  string        `json:"language"`
	TimeLimit time.Duration `json:"timeLimit"`
	MemLimit  int64         `json:"memLimit"`
	Input     string        `json:"input"`
}

// ExecResult 执行结果
type ExecResult struct {
	TaskId   string        `json:"taskId,omitempty"`
	Status   string        `json:"status"`
	Output   string        `json:"output"`
	Error    string        `json:"error"`
	TimeUsed time.Duration `json:"timeUsed"`
	MemUsed  int64         `json:"memUsed"`
}

// Executor 语言执行器接口
type Executor interface {
	Compile() error
	Execute() (*ExecResult, error)
	Cleanup()
}

// BaseExecutor 基础执行器
type BaseExecutor struct {
	config  ExecConfig
	workDir string
}

// ==================== Redis 无状态判题任务 ====================

// JudgeTask 从 Redis 获取的判题任务
type JudgeTask struct {
	TaskId    string `json:"taskId"`
	Code      string `json:"code"`
	Language  string `json:"language"`
	Input     string `json:"input"`
	TimeLimit int64  `json:"timeLimit"` // 毫秒
	MemLimit  int64  `json:"memLimit"`  // MB
}

// JudgeResult 判题结果（存入 Redis）
type JudgeResult struct {
	TaskId   string `json:"taskId"`
	Status   string `json:"status"`
	Output   string `json:"output"`
	Error    string `json:"error"`
	TimeUsed int64  `json:"timeUsed"` // 毫秒
	MemUsed  int64  `json:"memUsed"`  // KB
}

// JudgeRequest 服务端发来的判题请求
type JudgeRequest struct {
	TaskId string `json:"taskId"`
}

// ==================== Docker 相关 ====================

// DockerContainerListReq 列出容器请求
type DockerContainerListReq struct {
	All bool `json:"all"`
}

// DockerContainerOpReq 容器操作请求（启动/停止/删除）
type DockerContainerOpReq struct {
	ContainerId string `json:"containerId"`
}

// DockerContainerExecReq 容器内执行命令请求
type DockerContainerExecReq struct {
	ContainerId string   `json:"containerId"`
	Cmd         []string `json:"cmd"`
}

// DockerContainerCreateReq 创建容器请求
type DockerContainerCreateReq struct {
	Image string   `json:"image"`
	Cmd   []string `json:"cmd"`
	Name  string   `json:"name"`
}

// DockerImagePullReq 拉取镜像请求
type DockerImagePullReq struct {
	Image string `json:"image"`
}

// DockerImageRemoveReq 删除镜像请求
type DockerImageRemoveReq struct {
	Image string `json:"image"`
}

// DockerContainerInfo 容器信息（WebSocket 返回）
type DockerContainerInfo struct {
	ID      string `json:"id"`
	Names   string `json:"names"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Created int64  `json:"created"`
}

// DockerImageInfo 镜像信息（WebSocket 返回）
type DockerImageInfo struct {
	ID       string   `json:"id"`
	RepoTags []string `json:"repoTags"`
	Size     int64    `json:"size"`
	Created  int64    `json:"created"`
}
