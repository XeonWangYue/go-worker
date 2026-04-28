package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

// InitRedis 初始化 Redis 连接
func InitRedis(addr string, password string, db int) error {
	rdb = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("连接 Redis 失败: %w", err)
	}
	log.Println("Redis 连接成功:", addr)
	return nil
}

const (
	judgeQueueKey  = "judge:queue"   // 判题任务队列（List）
	judgeResultKey = "judge:result:" // 判题结果前缀，后接 taskId
)

// PollJudgeTask 从 Redis 阻塞获取判题任务
func PollJudgeTask() (*JudgeTask, error) {
	ctx := context.Background()

	// BRPop 阻塞等待任务，超时 5 秒
	result, err := rdb.BRPop(ctx, 5*time.Second, judgeQueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 超时无任务
		}
		return nil, err
	}

	// result[0] 是 key 名，result[1] 是 value
	var task JudgeTask
	if err := json.Unmarshal([]byte(result[1]), &task); err != nil {
		return nil, fmt.Errorf("解析判题任务失败: %w", err)
	}
	return &task, nil
}

// StoreJudgeResult 将判题结果存入 Redis
func StoreJudgeResult(result *JudgeResult) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化判题结果失败: %w", err)
	}

	key := judgeResultKey + result.TaskId
	// 结果保留 1 小时
	return rdb.Set(ctx, key, data, 1*time.Hour).Err()
}

// RunJudgeLoop 持续从 Redis 拉取判题任务并执行
func RunJudgeLoop(wsConn *WSConnection) {
	log.Println("Redis 判题循环启动...")
	for {
		task, err := PollJudgeTask()
		if err != nil {
			log.Printf("获取判题任务失败: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if task == nil {
			continue // 超时，继续轮询
		}

		log.Printf("收到判题任务: %s", task.TaskId)
		go processJudgeTask(task, wsConn)
	}
}

// processJudgeTask 执行单个判题任务
func processJudgeTask(task *JudgeTask, wsConn *WSConnection) {
	config := ExecConfig{
		Code:      task.Code,
		Language:  task.Language,
		TimeLimit: time.Duration(task.TimeLimit) * time.Millisecond,
		MemLimit:  task.MemLimit,
		Input:     task.Input,
	}

	executor, err := NewExecutor(config)
	if err != nil {
		log.Printf("创建执行器失败 [%s]: %v", task.TaskId, err)
		storeJudgeError(task.TaskId, err)
		return
	}
	defer executor.Cleanup()

	execResult, err := executor.Execute()
	if err != nil {
		log.Printf("执行失败 [%s]: %v", task.TaskId, err)
		storeJudgeError(task.TaskId, err)
		return
	}

	judgeResult := &JudgeResult{
		TaskId:   task.TaskId,
		Status:   execResult.Status,
		Output:   execResult.Output,
		Error:    execResult.Error,
		TimeUsed: execResult.TimeUsed.Milliseconds(),
		MemUsed:  execResult.MemUsed,
	}

	if err := StoreJudgeResult(judgeResult); err != nil {
		log.Printf("存储判题结果失败 [%s]: %v", task.TaskId, err)
		return
	}

	log.Printf("判题完成 [%s]: %s, 耗时 %dms", task.TaskId, judgeResult.Status, judgeResult.TimeUsed)

	// 通过 WebSocket 通知服务端判题完成
	if wsConn != nil {
		wsConn.Send(WorkerMsg{
			MsgType: "judge_done",
			Data:    judgeResult,
		})
	}
}

// storeJudgeError 存储错误结果
func storeJudgeError(taskId string, err error) {
	result := &JudgeResult{
		TaskId: taskId,
		Status: "SE", // System Error
		Error:  err.Error(),
	}
	if e := StoreJudgeResult(result); e != nil {
		log.Printf("存储错误结果失败 [%s]: %v", taskId, e)
	}
}
