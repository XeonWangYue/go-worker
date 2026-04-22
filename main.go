package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"golang.org/x/net/context"
)

// 监控指标结构
type Metric struct {
	Timestamp int64   `json:"timestamp"`
	CPU       float64 `json:"cpu_usage"`
	MemTotal  uint64  `json:"mem_total"`
	MemUsed   uint64  `json:"mem_used"`
	DiskTotal uint64  `json:"disk_total"`
	DiskUsed  uint64  `json:"disk_used"`
	NetSent   uint64  `json:"net_sent_bytes"`
	NetRecv   uint64  `json:"net_recv_bytes"`
}

func main() {
	log.Println("=== 轻量无状态监控+沙盒Worker启动 ===")

	// 1. 初始化Docker客户端
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal("Docker客户端初始化失败:", err)
	}
	defer dockerCli.Close()

	ctx := context.Background()

	// 2. 启动监控采集协程
	go collectAndReportMetrics(ctx)

	// 3. 创建一个临时沙盒容器（示例：alpine，执行sleep 10s后自动销毁）
	go func() {
		for i := 0; i < 3; i++ { // 循环创建3个沙盒示例
			containerID, err := createSandbox(ctx, dockerCli)
			if err != nil {
				log.Println("创建沙盒失败:", err)
				time.Sleep(5 * time.Second)
				continue
			}
			log.Printf("沙盒创建成功 ID: %s", containerID[:12])

			// 等待后删除沙盒
			time.Sleep(15 * time.Second)
			if err := removeSandbox(ctx, dockerCli, containerID); err != nil {
				log.Println("删除沙盒失败:", err)
			} else {
				log.Printf("沙盒已清理: %s", containerID[:12])
			}
		}
	}()

	// 优雅退出
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Worker 退出")
}

// 采集监控并上报
func collectAndReportMetrics(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m, err := getMetrics()
		if err != nil {
			log.Println("采集指标失败:", err)
			continue
		}
		reportMetrics(m)
	}
}

// 获取服务器指标
func getMetrics() (*Metric, error) {
	cpuPercent, err := cpu.Percent(2*time.Second, false)
	if err != nil || len(cpuPercent) == 0 {
		return nil, err
	}

	memInfo, _ := mem.VirtualMemory()
	diskInfo, _ := disk.Usage("/")
	netInfo, _ := net.IOCounters(false)

	m := &Metric{
		Timestamp: time.Now().Unix(),
		CPU:       cpuPercent[0],
		MemTotal:  memInfo.Total,
		MemUsed:   memInfo.Used,
		DiskTotal: diskInfo.Total,
		DiskUsed:  diskInfo.Used,
	}

	if len(netInfo) > 0 {
		m.NetSent = netInfo[0].BytesSent
		m.NetRecv = netInfo[0].BytesRecv
	}
	return m, nil
}

// 上报到中心服务（替换成你的接口）
func reportMetrics(m *Metric) {
	bs, _ := json.Marshal(m)
	log.Printf("采集指标: CPU=%.1f%% 内存=%dMB", m.CPU, m.MemUsed/1024/1024)

	// 示例上报（可替换为gRPC/NATS）
	resp, err := http.Post(
		"https://httpbin.org/post",
		"application/json",
		bytes.NewBuffer(bs),
	)
	if err != nil {
		log.Println("上报失败:", err)
		return
	}
	defer resp.Body.Close()
	log.Println("上报成功, 状态码:", resp.StatusCode)
}

// 创建沙盒容器
func createSandbox(ctx context.Context, cli *client.Client) (string, error) {
	// 拉取镜像（如果没有）
	imageName := "alpine:latest"
	_, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return "", err
	}

	// 创建容器：轻量、隔离、用完即删
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:           imageName,
		Cmd:             []string{"sleep", "10"},
		WorkingDir:      "/sandbox",
		NetworkDisabled: true, // 加强隔离
	}, &container.HostConfig{
		AutoRemove: true, // 退出自动删除
		Resources: container.Resources{
			Memory:    64 * 1024 * 1024, // 64MB
			CPUPeriod: 100000,
			CPUQuota:  20000, // 20% CPU
			PidsLimit: 64,
		},
	}, nil, nil, "")
	if err != nil {
		return "", err
	}

	// 启动容器
	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// 删除沙盒
func removeSandbox(ctx context.Context, cli *client.Client, id string) error {
	return cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}
