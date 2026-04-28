package main

import (
	"log"
	"os/exec"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	reportInterval    = 1 * time.Second
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
	reportChan = make(chan ReportMsg, 1)

	// IO 基准值：只在采集协程使用，无竞争
	lastNet      psnet.IOCountersStat
	lastDisk     disk.IOCountersStat
	lastTime     time.Time
	netInterface string
)

// collectTask 定时采集系统信息并发送到 reportChan
func collectTask() {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	for range ticker.C {
		msg := collectAll()
		select {
		case reportChan <- msg:
		default:
			// 防止发送慢导致阻塞，直接丢弃旧数据
		}
	}
}

// collectAll 采集所有系统信息
func collectAll() ReportMsg {
	now := time.Now()
	msg := ReportMsg{Time: now.Format("2006-01-02 15:04:05.000")}

	if reportHostCpu {
		cores, _ := cpu.Percent(0, true)
		for _, v := range cores {
			msg.HostCPU = append(msg.HostCPU, round(v))
		}
	}

	if msg.HostCPU == nil {
		msg.HostCPU = make([]float64, 0)
	}

	if reportHostMem {
		memInfo, _ := mem.VirtualMemory()
		msg.HostMem = round(memInfo.UsedPercent)
	}

	if reportHostNet {
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

	// 子进程统计（只读，不修改）
	for pid := range processTable {
		p, err := process.NewProcess(int32(pid))
		if err != nil {
			continue
		}
		cpuVal, _ := p.CPUPercent()
		memVal, _ := p.MemoryPercent()
		msg.Processes = append(msg.Processes, ProcessStat{
			Pid: pid, CpuUsed: round(cpuVal), MemUsed: round(float64(memVal)),
		})
	}

	if msg.Processes == nil {
		msg.Processes = make([]ProcessStat, 0)
	}

	lastTime = now
	return msg
}

// sendTask 从 reportChan 读取并通过 WebSocket 发送
func sendTask(wsConn *WSConnection) {
	for msg := range reportChan {
		rpt := WorkerMsg{MsgType: "monitor", Data: msg}
		_ = wsConn.Send(rpt)
		log.Printf("上报成功：%d 个进程", len(msg.Processes))
	}
}
