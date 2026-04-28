package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

var dockerCli *client.Client

// InitDocker 初始化 Docker 客户端
func InitDocker() error {
	var err error
	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("初始化 Docker 客户端失败: %w", err)
	}
	return nil
}

// ==================== 容器操作 ====================

// DockerListContainers 列出容器
func DockerListContainers(all bool) ([]DockerContainerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := types.ContainerListOptions{All: all}
	containers, err := dockerCli.ContainerList(ctx, opts)
	if err != nil {
		return nil, err
	}

	var result []DockerContainerInfo
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		result = append(result, DockerContainerInfo{
			ID:      c.ID,
			Names:   name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Created: c.Created,
		})
	}
	return result, nil
}

// DockerStartContainer 启动容器
func DockerStartContainer(containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return dockerCli.ContainerStart(ctx, containerID, types.ContainerStartOptions{})
}

// DockerStopContainer 停止容器
func DockerStopContainer(containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	timeout := 10
	stopOpts := container.StopOptions{Timeout: &timeout}
	return dockerCli.ContainerStop(ctx, containerID, stopOpts)
}

// DockerRemoveContainer 删除容器
func DockerRemoveContainer(containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return dockerCli.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
}

// DockerCreateContainer 创建容器
func DockerCreateContainer(req DockerContainerCreateReq) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := &container.Config{
		Image: req.Image,
	}
	if len(req.Cmd) > 0 {
		cfg.Cmd = req.Cmd
	}

	hostCfg := &container.HostConfig{}
	resp, err := dockerCli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, req.Name)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// DockerExecInContainer 在容器中执行命令
func DockerExecInContainer(containerID string, cmd []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	execCfg := types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := dockerCli.ContainerExecCreate(ctx, containerID, execCfg)
	if err != nil {
		return "", err
	}

	attachResp, err := dockerCli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", err
	}
	defer attachResp.Close()

	output, err := io.ReadAll(attachResp.Reader)
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// ==================== 镜像操作 ====================

// DockerListImages 列出镜像
func DockerListImages() ([]DockerImageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	images, err := dockerCli.ImageList(ctx, types.ImageListOptions{})
	if err != nil {
		return nil, err
	}

	var result []DockerImageInfo
	for _, img := range images {
		result = append(result, DockerImageInfo{
			ID:       img.ID,
			RepoTags: img.RepoTags,
			Size:     img.Size,
			Created:  img.Created,
		})
	}
	return result, nil
}

// DockerPullImage 拉取镜像
func DockerPullImage(imageName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	reader, err := dockerCli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()

	// 读取并丢弃 pull 输出（必须读完才能完成）
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// DockerRemoveImage 删除镜像
func DockerRemoveImage(imageName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := dockerCli.ImageRemove(ctx, imageName, types.ImageRemoveOptions{Force: true})
	return err
}

// ==================== 命令分发 ====================

// HandleDockerCommand 处理来自 WebSocket 的 Docker 命令
func HandleDockerCommand(wsConn *WSConnection, msgType string, data json.RawMessage) {
	switch msgType {
	case "docker_container_list":
		var req DockerContainerListReq
		json.Unmarshal(data, &req)
		containers, err := DockerListContainers(req.All)
		sendDockerResult(wsConn, "docker_container_list_result", containers, err)

	case "docker_container_start":
		var req DockerContainerOpReq
		json.Unmarshal(data, &req)
		err := DockerStartContainer(req.ContainerId)
		sendDockerResult(wsConn, "docker_container_start_result", nil, err)

	case "docker_container_stop":
		var req DockerContainerOpReq
		json.Unmarshal(data, &req)
		err := DockerStopContainer(req.ContainerId)
		sendDockerResult(wsConn, "docker_container_stop_result", nil, err)

	case "docker_container_remove":
		var req DockerContainerOpReq
		json.Unmarshal(data, &req)
		err := DockerRemoveContainer(req.ContainerId)
		sendDockerResult(wsConn, "docker_container_remove_result", nil, err)

	case "docker_container_create":
		var req DockerContainerCreateReq
		json.Unmarshal(data, &req)
		id, err := DockerCreateContainer(req)
		sendDockerResult(wsConn, "docker_container_create_result", map[string]string{"containerId": id}, err)

	case "docker_container_exec":
		var req DockerContainerExecReq
		json.Unmarshal(data, &req)
		output, err := DockerExecInContainer(req.ContainerId, req.Cmd)
		sendDockerResult(wsConn, "docker_container_exec_result", map[string]string{"output": output}, err)

	case "docker_image_list":
		images, err := DockerListImages()
		sendDockerResult(wsConn, "docker_image_list_result", images, err)

	case "docker_image_pull":
		var req DockerImagePullReq
		json.Unmarshal(data, &req)
		err := DockerPullImage(req.Image)
		sendDockerResult(wsConn, "docker_image_pull_result", nil, err)

	case "docker_image_remove":
		var req DockerImageRemoveReq
		json.Unmarshal(data, &req)
		err := DockerRemoveImage(req.Image)
		sendDockerResult(wsConn, "docker_image_remove_result", nil, err)

	default:
		log.Printf("未知的 Docker 命令: %s", msgType)
	}
}

func sendDockerResult(wsConn *WSConnection, msgType string, data interface{}, err error) {
	if err != nil {
		wsConn.Send(WorkerMsg{
			MsgType: msgType,
			Data:    map[string]string{"error": err.Error()},
		})
	} else {
		wsConn.Send(WorkerMsg{
			MsgType: msgType,
			Data:    data,
		})
	}
}
