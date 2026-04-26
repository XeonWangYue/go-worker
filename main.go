package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"os/exec"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	reportInterval    = 1 * time.Second // 统一每秒上报 	// 网卡名
	reportHostCpu     = true
	reportHostMem     = true
	reportHostDisk    = false
	reportHostNet     = false
	reportHostProcess = true
	reportHostThreads = true
)

var (
	// 子进程注册表：只在 主协程/指令协程 操作，无并发
	processTable = make(map[int]*exec.Cmd)

	// 通道：采集协程 → 发送协程（无锁传递）
	reportChan = make(chan ReportMsg, 1) // 带缓冲，不阻塞

	// IO 基准值：只在采集协程使用，无竞争
	lastNet      psnet.IOCountersStat
	lastDisk     disk.IOCountersStat
	lastTime     time.Time
	netInterface string
)

type ServerMsg struct {
	MsgType string      `json:"msgType"` // create / kill
	Data    interface{} `json:"data"`
}

type WorkerMsg struct {
	MsgType string      `json:"msgType"`
	Data    interface{} `json:"data"`
}

type CreateCmdData struct {
	Command string `json:"command"`
}

type KillCmdData struct {
	Pid int `json:"pid"`
}

type ProcessStat struct {
	Pid     int     `json:"pid"`
	CpuUsed float64 `json:"cpuUsed"`
	MemUsed float64 `json:"memUsed"`
}

// 统一上报消息：主机 + 所有子进程（一条消息发完）
type ReportMsg struct {
	Time           string        `json:"time"`
	HostCPU        []float64     `json:"hostCpu"`        // 所有CPU核心
	HostMem        float64       `json:"hostMem"`        // 内存使用率
	NetSend        uint64        `json:"netSend"`        // 上传 B/s
	NetRecv        uint64        `json:"netRecv"`        // 下载 B/s
	DiskRead       uint64        `json:"diskRead"`       // 磁盘读
	DiskWrite      uint64        `json:"diskWrite"`      // 磁盘写
	NetSendStr     string        `json:"netSendStr"`     // 上传
	NetRecvStr     string        `json:"netRecvStr"`     // 下载
	DiskReadStr    string        `json:"diskReadStr"`    // 磁盘读
	DiskWriteStr   string        `json:"diskWriteStr"`   // 磁盘写
	TotalProcesses int           `json:"totalProcesses"` // 系统总进程数
	TotalThreads   int           `json:"totalThreads"`   // 系统总线程数
	Processes      []ProcessStat `json:"processes"`      // 子进程列表
}

// 执行配置：用户代码、语言、时间/内存限制、输入数据
type ExecConfig struct {
	Code      string        `json:"code"`      // 用户代码
	Language  string        `json:"language"`  // 语言：c/cpp/python/java
	TimeLimit time.Duration `json:"timeLimit"` // 时间限制（毫秒）
	MemLimit  int64         `json:"memLimit"`  // 内存限制（MB）
	Input     string        `json:"input"`     // 标准输入
}

// 执行结果：统一返回格式
type ExecResult struct {
	Status   string        `json:"status"`   // 状态：AC/CE/TLE/MLE/RE
	Output   string        `json:"output"`   // 标准输出
	Error    string        `json:"error"`    // 错误信息
	TimeUsed time.Duration `json:"timeUsed"` // 实际耗时
	MemUsed  int64         `json:"memUsed"`  // 实际内存(KB)
}

// 语言执行器接口
type Executor interface {
	Compile() error                // 编译（解释型语言为空）
	Execute() (*ExecResult, error) // 运行
	Cleanup()                      // 清理临时文件
}

// 基础执行器：所有语言共用
type BaseExecutor struct {
	config  ExecConfig
	workDir string // 临时工作目录
}

func main() {
	// WebSocket 服务端地址
	port := flag.String("port", "8080", "服务端口")
	url := flag.String("url", "ws://127.0.0.1", "Web Socket连接地址")
	path := flag.String("path", "/ws/jserver", "Web Socket路径")

	flag.Parse()

	fp := fmt.Sprintf("%s:%s%s", *url, *port, *path)
	log.Println(fp)
	// 1. 建立连接
	conn, _, err := websocket.DefaultDialer.Dial(fp, nil)

	if err != nil {
		log.Fatal("连接失败:", err)
	}
	defer conn.Close()

	localAddr, ok := conn.NetConn().LocalAddr().(*net.TCPAddr)
	localIP := localAddr.IP.String()
	log.Println(localIP)

	netInterface, _ = GetInterfaceByIP(localIP)

	if !ok {
		panic("不是TCP连接")
	}
	log.Println("连接成功")

	initIO()

	go collectTask()

	// 3. 启动协程异步定时发送消息
	go sendTask(conn)

	// 3. 读取服务端消息
	DispatchServerMessage(conn)
}

func DispatchServerMessage(conn *websocket.Conn) {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("读取失败:", err)
			return
		}
		log.Printf("收到服务端消息: %s", msg)
	}
}

func sendTask(conn *websocket.Conn) {
	for msg := range reportChan {
		rpt := WorkerMsg{MsgType: "monitor", Data: msg}
		_ = conn.WriteJSON(rpt)
		log.Printf("上报成功：%d 个进程", len(msg.Processes))
	}
}

func collectTask() {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	for range ticker.C {
		msg := collectAll()
		// 无锁：直接发给发送协程
		select {
		case reportChan <- msg:
		default:
			// 防止发送慢导致阻塞，直接丢弃旧数据
		}
	}
}

func collectAll() ReportMsg {
	now := time.Now()
	msg := ReportMsg{Time: now.Format("2006-01-02 15:04:05.000")}

	if reportHostCpu {
		// 1. CPU 每个核心使用率
		cores, _ := cpu.Percent(0, true)
		for _, v := range cores {
			msg.HostCPU = append(msg.HostCPU, round(v))
		}
	}

	if msg.HostCPU == nil {
		msg.HostCPU = make([]float64, 0)
	}

	if reportHostMem {
		// 2. 内存
		memInfo, _ := mem.VirtualMemory()
		msg.HostMem = round(memInfo.UsedPercent)
	}
	if reportHostNet {
		// 3. 网络
		netStats, _ := psnet.IOCounters(true)
		for _, v := range netStats {
			if v.Name == netInterface {
				dt := now.Sub(lastTime).Seconds()
				msg.NetSend = uint64(float64(v.BytesSent-lastNet.BytesSent) / dt)
				msg.NetRecv = uint64(float64(v.BytesRecv-lastNet.BytesRecv) / dt)
				msg.NetSendStr = FormatBytes(msg.NetSend)
				msg.NetRecvStr = FormatBytes(msg.NetRecv)
				lastNet = v
				break
			}
		}

	}
	if reportHostDisk {
		// 4. 磁盘IO
		diskStats, _ := disk.IOCounters()
		for _, v := range diskStats {
			dt := now.Sub(lastTime).Seconds()
			msg.DiskRead = uint64(float64(v.ReadBytes-lastDisk.ReadBytes) / dt)
			msg.DiskWrite = uint64(float64(v.WriteBytes-lastDisk.WriteBytes) / dt)
			msg.DiskReadStr = FormatBytes(msg.DiskRead)
			msg.DiskWriteStr = FormatBytes(msg.DiskWrite)
			lastDisk = v
			break
		}
	}
	if reportHostProcess || reportHostThreads {
		// 6. 系统总进程数和总线程数
		allProcesses, _ := process.Processes()

		if reportHostProcess {
			msg.TotalProcesses = len(allProcesses)
		}

		if reportHostThreads {
			totalThreads := 0
			for _, p := range allProcesses {
				tids, _ := p.Threads()
				totalThreads += len(tids)
			}
			msg.TotalThreads = totalThreads
		}
	}

	// 6. 【无锁读取】子进程（只读，不修改）
	for pid, cmd := range processTable {
		if cmd.Process == nil {
			continue
		}
		p, err := process.NewProcess(int32(pid))
		if err != nil {
			continue
		}
		cpu, _ := p.CPUPercent()
		mem, _ := p.MemoryPercent()
		msg.Processes = append(msg.Processes, ProcessStat{
			Pid: pid, CpuUsed: round(cpu), MemUsed: round(float64(mem)),
		})
	}

	if msg.Processes == nil {
		msg.Processes = make([]ProcessStat, 0)
	}

	lastTime = now

	return msg
}

// ===================== 工具 =====================
func initIO() {
	netStats, _ := psnet.IOCounters(true)
	for _, v := range netStats {
		if v.Name == netInterface {
			lastNet = v
			break
		}
	}
	diskStats, _ := disk.IOCounters()
	for _, v := range diskStats {
		lastDisk = v
		break
	}
	lastTime = time.Now()
}

func round(v float64) float64 {
	return float64(int(v*100)) / 100
}

func GetInterfaceByIP(targetIP string) (string, error) {
	ifaces, err := psnet.Interfaces()
	if err != nil {
		return "", err
	}

	// 遍历网卡，找匹配IP的那一个
	for _, iface := range ifaces {
		for _, addr := range iface.Addrs {
			// 只比对IP地址，自动忽略掩码
			if addr.Addr == targetIP {
				return iface.Name, nil
			}
		}
	}

	return "", fmt.Errorf("未找到IP对应的网卡")
}

func FormatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%.2f B", float64(bytes))
	}
	values := []string{"B", "KB", "MB", "GB", "TB"}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit && exp < len(values)-2; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %s", float64(bytes)/float64(div), values[exp+1])
}
