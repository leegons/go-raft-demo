package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/leegons/go-raft-demo/raft"
)

func main() {
	fmt.Println("=== Raft 分布式共识算法 Demo ===")
	fmt.Println("创建一个包含 5 个节点的 Raft 集群\n")

	// 创建 5 个 Raft 节点
	numNodes := 5
	nodes := make([]*raft.Raft, numNodes)

	// 先创建所有节点（互相引用）
	for i := 0; i < numNodes; i++ {
		nodes[i] = raft.NewRaft(nil, i)
	}

	// 设置 peers 引用
	for i := 0; i < numNodes; i++ {
		nodes[i] = raft.NewRaft(nodes, i)
	}

	fmt.Printf("✅ 已创建 %d 个 Raft 节点\n", numNodes)
	fmt.Println("等待选举产生 Leader...\n")

	// 等待选举完成
	time.Sleep(500 * time.Millisecond)

	// 显示各节点状态
	fmt.Println("=== 节点状态 ===")
	for i, node := range nodes {
		state, term := node.GetState()
		fmt.Printf("Node %d: %s (Term: %d)\n", i, state, term)
	}
	fmt.Println()

	// 找到 Leader
	var leader *raft.Raft
	leaderId := -1
	for i, node := range nodes {
		state, _ := node.GetState()
		if state == raft.Leader {
			leader = node
			leaderId = i
			break
		}
	}

	if leader == nil {
		fmt.Println("❌ 未选出 Leader，请稍后重试")
		return
	}

	fmt.Printf("✅ Node %d 成为 Leader\n\n", leaderId)

	// 交互式命令行
	fmt.Println("=== 命令帮助 ===")
	fmt.Println("  submit <消息>  - 提交命令到 Raft 集群")
	fmt.Println("  status         - 查看所有节点状态")
	fmt.Println("  log            - 查看 Leader 日志")
	fmt.Println("  quit           - 退出程序")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("raft> ")
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
			goto cleanup
		case "status":
			fmt.Println("\n=== 节点状态 ===")
			for i, node := range nodes {
				state, term := node.GetState()
				log := node.GetLog()
				fmt.Printf("Node %d: %-10s Term: %d, 日志条目：%d, 已提交：%d\n",
					i, state, term, len(log), getCommitIndex(node))
			}
			fmt.Println()

		case "log":
			if leader == nil {
				fmt.Println("❌ 当前没有 Leader")
				break
			}
			log := leader.GetLog()
			fmt.Println("\n=== Leader 日志 ===")
			if len(log) == 0 {
				fmt.Println("(空)")
			} else {
				for _, entry := range log {
					fmt.Printf("索引 %d, Term %d: %v\n", entry.Index, entry.Term, entry.Command)
				}
			}
			fmt.Println()

		case "submit":
			if len(parts) < 2 {
				fmt.Println("用法：submit <消息>")
				break
			}
			message := parts[1]

			// 重新检查 Leader（可能已变更）
			leader = nil
			for i, node := range nodes {
				state, _ := node.GetState()
				if state == raft.Leader {
					leader = node
					leaderId = i
					break
				}
			}

			if leader == nil {
				fmt.Println("❌ 当前没有 Leader，无法提交命令")
				break
			}

			if leader.Submit(message) {
				fmt.Printf("✅ 命令已提交到 Node %d (Leader)\n", leaderId)
				// 等待日志复制
				time.Sleep(200 * time.Millisecond)
			} else {
				fmt.Println("❌ 提交失败，当前节点不是 Leader")
			}

		default:
			fmt.Printf("未知命令：%s\n", cmd)
			fmt.Println("输入 'status', 'log', 'submit <消息>', 或 'quit'")
		}
	}

cleanup:
	// 清理资源
	fmt.Println("停止所有节点...")
	for _, node := range nodes {
		node.Stop()
	}
	time.Sleep(100 * time.Millisecond)
}

// getCommitIndex 获取节点的 commitIndex（通过反射或公开方法）
// 这里简化处理，返回日志长度
func getCommitIndex(node *raft.Raft) int {
	log := node.GetLog()
	return len(log)
}
