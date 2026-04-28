package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// NewExecutor 创建对应语言的执行器
func NewExecutor(config ExecConfig) (Executor, error) {
	workDir, err := os.MkdirTemp("", "oj-sandbox-*")
	if err != nil {
		return nil, err
	}

	base := &BaseExecutor{
		config:  config,
		workDir: workDir,
	}

	codeFile := filepath.Join(workDir, getCodeFileName(config.Language))
	if err := os.WriteFile(codeFile, []byte(config.Code), 0644); err != nil {
		return nil, err
	}

	switch strings.ToLower(config.Language) {
	case "c":
		return &CExecutor{BaseExecutor: base}, nil
	case "cpp":
		return &CppExecutor{BaseExecutor: base}, nil
	case "python":
		return &PythonExecutor{BaseExecutor: base}, nil
	case "java":
		return &JavaExecutor{BaseExecutor: base}, nil
	default:
		return nil, errors.New("不支持的语言")
	}
}

func getCodeFileName(lang string) string {
	switch lang {
	case "c":
		return "main.c"
	case "cpp":
		return "main.cpp"
	case "python":
		return "main.py"
	case "java":
		return "Main.java"
	default:
		return "main"
	}
}

// ===================== C 执行器 =====================
type CExecutor struct{ *BaseExecutor }

func (e *CExecutor) Compile() error {
	cmd := exec.Command("gcc", "-o", filepath.Join(e.workDir, "main"), filepath.Join(e.workDir, "main.c"), "-lm")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("编译失败: %s", string(output))
	}
	return nil
}

// ===================== C++ 执行器 =====================
type CppExecutor struct{ *BaseExecutor }

func (e *CppExecutor) Compile() error {
	cmd := exec.Command("g++", "-o", filepath.Join(e.workDir, "main"), filepath.Join(e.workDir, "main.cpp"), "-lm", "-std=c++17")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("编译失败: %s", string(output))
	}
	return nil
}

// ===================== Python 执行器 =====================
type PythonExecutor struct{ *BaseExecutor }

func (e *PythonExecutor) Compile() error { return nil }

// ===================== Java 执行器 =====================
type JavaExecutor struct{ *BaseExecutor }

func (e *JavaExecutor) Compile() error {
	cmd := exec.Command("javac", filepath.Join(e.workDir, "Main.java"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("编译失败: %s", string(output))
	}
	return nil
}

// ===================== 统一运行逻辑 =====================
func (e *BaseExecutor) Execute() (*ExecResult, error) {
	result := &ExecResult{Status: "AC"}

	executor, _ := NewExecutor(e.config)
	if err := executor.Compile(); err != nil {
		result.Status = "CE"
		result.Error = err.Error()
		return result, nil
	}

	var cmd *exec.Cmd
	switch strings.ToLower(e.config.Language) {
	case "c", "cpp":
		cmd = exec.Command("./main")
	case "python":
		cmd = exec.Command("python", "main.py")
	case "java":
		cmd = exec.Command("java", "-Xmx"+fmt.Sprintf("%dm", e.config.MemLimit), "Main")
	}
	cmd.Dir = e.workDir

	cmd.Stdin = strings.NewReader(e.config.Input)

	ctx, cancel := context.WithTimeout(context.Background(), e.config.TimeLimit*time.Millisecond)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmd.Dir = e.workDir
	cmd.Stdin = strings.NewReader(e.config.Input)

	cmd.SysProcAttr = &syscall.SysProcAttr{}

	startTime := time.Now()
	output, err := cmd.CombinedOutput()
	result.TimeUsed = time.Since(startTime)

	if ctx.Err() == context.DeadlineExceeded {
		result.Status = "TLE"
		result.Error = "执行超时"
		return result, nil
	}

	if err != nil {
		result.Status = "RE"
		result.Error = err.Error() + "\n" + string(output)
		return result, nil
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	result.MemUsed = int64(memStats.HeapInuse / 1024)

	result.Output = string(output)
	return result, nil
}

// Cleanup 清理临时目录
func (e *BaseExecutor) Cleanup() {
	_ = os.RemoveAll(e.workDir)
}

// ExecCode 执行代码并输出结果（用于 WebSocket 直接下发的代码执行）
func ExecCode(config ExecConfig) {
	executor, err := NewExecutor(config)
	if err != nil {
		log.Printf("创建执行器失败：%v\n", err)
		return
	}
	defer executor.Cleanup()

	result, err := executor.Execute()
	if err != nil {
		log.Printf("执行失败：%v\n", err)
		return
	}

	jsonStr, _ := json.MarshalIndent(result, "", "  ")
	log.Println("执行结果：")
	log.Println(string(jsonStr))
}
