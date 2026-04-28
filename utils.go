package main

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	psnet "github.com/shirou/gopsutil/v3/net"
)

// round 保留两位小数
func round(v float64) float64 {
	return float64(int(v*100)) / 100
}

// FormatBytes 格式化字节数为可读字符串
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

// GetInterfaceByIP 根据 IP 地址获取网卡名
func GetInterfaceByIP(targetIP string) (string, error) {
	ifaces, err := psnet.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		for _, addr := range iface.Addrs {
			if addr.Addr == targetIP {
				return iface.Name, nil
			}
		}
	}
	return "", fmt.Errorf("未找到IP对应的网卡")
}

// initIO 初始化磁盘和网络 IO 基准值
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
