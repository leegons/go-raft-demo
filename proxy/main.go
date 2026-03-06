package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProxyService 代理服务
type ProxyService struct {
	masterAddr string
	httpClient *http.Client
}

// NewProxyService 创建代理服务
func NewProxyService(masterAddr string) *ProxyService {
	return &ProxyService{
		masterAddr: masterAddr,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetNodes 获取节点列表
func (p *ProxyService) GetNodes() ([]map[string]interface{}, error) {
	resp, err := p.httpClient.Get(fmt.Sprintf("http://%s/api/nodes", p.masterAddr))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Nodes []map[string]interface{} `json:"nodes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Nodes, nil
}

// GetLeader 获取 Leader 节点
func (p *ProxyService) GetLeader() (string, error) {
	resp, err := p.httpClient.Get(fmt.Sprintf("http://%s/api/raft-state", p.masterAddr))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var state map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return "", err
	}

	// 查找 Leader 节点
	for key, value := range state {
		if strings.Contains(key, "_state") {
			if nodeState, ok := value.(map[string]interface{}); ok {
				if stateStr, ok := nodeState["state"].(string); ok && stateStr == "Leader" {
					// 从 key 中提取节点 ID
					parts := strings.Split(key, "_")
					if len(parts) >= 2 {
						return parts[1], nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("未找到 Leader 节点")
}

// ForwardCommand 转发命令到 Leader 节点
func (p *ProxyService) ForwardCommand(nodeAddr string, command interface{}) error {
	req := map[string]interface{}{
		"command": command,
	}

	jsonData, _ := json.Marshal(req)
	resp, err := p.httpClient.Post(
		fmt.Sprintf("http://%s/api/submit", nodeAddr),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("请求失败：%s", string(body))
	}

	return nil
}

// GetNodeLog 获取节点日志
func (p *ProxyService) GetNodeLog(nodeAddr string) ([]map[string]interface{}, error) {
	resp, err := p.httpClient.Get(fmt.Sprintf("http://%s/api/log", nodeAddr))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Log []map[string]interface{} `json:"log"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Log, nil
}

// GetClusterStatus 获取集群状态
func (p *ProxyService) GetClusterStatus() (map[string]interface{}, error) {
	// 获取节点列表
	nodes, err := p.GetNodes()
	if err != nil {
		return nil, err
	}

	// 获取 Raft 状态
	resp, err := p.httpClient.Get(fmt.Sprintf("http://%s/api/raft-state", p.masterAddr))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var raftState map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raftState); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"nodes":      nodes,
		"node_count": len(nodes),
		"raft_state": raftState,
	}, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法：proxy <master_addr>")
		fmt.Println("示例：proxy localhost:8080")
		os.Exit(1)
	}

	masterAddr := os.Args[1]
	proxy := NewProxyService(masterAddr)

	fmt.Println("=== Raft 集群代理 ===")
	fmt.Printf("Master: %s\n\n", masterAddr)

	// 等待集群初始化
	fmt.Println("等待集群初始化...")
	time.Sleep(2 * time.Second)

	// 交互式命令行
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== 命令帮助 ===")
	fmt.Println("  status    - 查看集群状态")
	fmt.Println("  nodes     - 查看节点列表")
	fmt.Println("  leader    - 查看 Leader 节点")
	fmt.Println("  submit <消息> - 提交命令到集群")
	fmt.Println("  log       - 查看 Leader 日志")
	fmt.Println("  quit      - 退出程序")
	fmt.Println()

	for {
		fmt.Print("proxy> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		input = strings.TrimSpace(input)
		parts := strings.SplitN(input, " ", 2)
		cmd := parts[0]

		switch cmd {
		case "quit", "exit", "q":
			fmt.Println("退出程序...")
			return

		case "status":
			status, err := proxy.GetClusterStatus()
			if err != nil {
				fmt.Printf("❌ 获取状态失败：%v\n", err)
				break
			}
			fmt.Println("\n=== 集群状态 ===")
			fmt.Printf("节点数：%v\n", status["node_count"])
			fmt.Println("\n节点列表:")
			if nodes, ok := status["nodes"].([]map[string]interface{}); ok {
				for _, node := range nodes {
					fmt.Printf("  - %s (%s) [%s]\n", node["id"], node["address"], node["state"])
				}
			}
			fmt.Println()

		case "nodes":
			nodes, err := proxy.GetNodes()
			if err != nil {
				fmt.Printf("❌ 获取节点列表失败：%v\n", err)
				break
			}
			fmt.Println("\n=== 节点列表 ===")
			for _, node := range nodes {
				fmt.Printf("ID: %s, 地址：%s, 状态：%s\n",
					node["id"], node["address"], node["state"])
			}
			fmt.Println()

		case "leader":
			leaderID, err := proxy.GetLeader()
			if err != nil {
				fmt.Printf("❌ 获取 Leader 失败：%v\n", err)
				break
			}
			fmt.Printf("\n✅ Leader: %s\n\n", leaderID)

		case "submit":
			if len(parts) < 2 {
				fmt.Println("用法：submit <消息>")
				break
			}
			message := parts[1]

			leaderID, err := proxy.GetLeader()
			if err != nil {
				fmt.Printf("❌ 未找到 Leader：%v\n", err)
				break
			}

			// 获取 Leader 地址
			nodes, _ := proxy.GetNodes()
			leaderAddr := ""
			for _, node := range nodes {
				if node["id"] == leaderID {
					leaderAddr = node["address"].(string)
					break
				}
			}

			if leaderAddr == "" {
				fmt.Println("❌ 无法获取 Leader 地址")
				break
			}

			if err := proxy.ForwardCommand(leaderAddr, message); err != nil {
				fmt.Printf("❌ 提交失败：%v\n", err)
			} else {
				fmt.Printf("✅ 命令已提交到 Leader (%s)\n", leaderID)
			}
			fmt.Println()

		case "log":
			leaderID, err := proxy.GetLeader()
			if err != nil {
				fmt.Printf("❌ 未找到 Leader：%v\n", err)
				break
			}

			// 获取 Leader 地址
			nodes, _ := proxy.GetNodes()
			leaderAddr := ""
			for _, node := range nodes {
				if node["id"] == leaderID {
					leaderAddr = node["address"].(string)
					break
				}
			}

			if leaderAddr == "" {
				fmt.Println("❌ 无法获取 Leader 地址")
				break
			}

			log, err := proxy.GetNodeLog(leaderAddr)
			if err != nil {
				fmt.Printf("❌ 获取日志失败：%v\n", err)
				break
			}

			fmt.Println("\n=== Leader 日志 ===")
			if len(log) == 0 {
				fmt.Println("(空)")
			} else {
				for _, entry := range log {
					fmt.Printf("索引 %s, Term %s: %v\n",
						entry["index"], entry["term"], entry["command"])
				}
			}
			fmt.Println()

		default:
			// 尝试解析为端口号，启动节点
			if port, err := strconv.Atoi(cmd); err == nil {
				addr := fmt.Sprintf("localhost:%d", port)
				fmt.Printf("🚀 启动节点，地址：%s\n", addr)
				// 这里只是示例，实际需要 fork 进程
				fmt.Println("(提示：在新终端运行：go run node/main.go nodeX localhost:8080 %s)\n", addr)
			} else {
				fmt.Printf("未知命令：%s\n", cmd)
				fmt.Println("输入 'status', 'nodes', 'leader', 'submit <消息>', 'log', 或 'quit'")
			}
		}
	}
}
