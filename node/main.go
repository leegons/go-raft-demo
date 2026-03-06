package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// NodeState 节点状态
type NodeState int

const (
	Follower NodeState = iota
	Candidate
	Leader
)

func (s NodeState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// LogEntry 日志条目
type LogEntry struct {
	Term    int         `json:"term"`
	Command interface{} `json:"command"`
	Index   int         `json:"index"`
}

// RaftNode Raft 节点
type RaftNode struct {
	mu sync.Mutex

	// 配置
	nodeID      string
	masterAddr  string
	selfAddr    string
	peers       []string

	// Raft 状态
	state       NodeState
	currentTerm int
	votedFor    string
	log         []LogEntry
	commitIndex int
	lastApplied int

	// 选举相关
	votesReceived int
	electionTimer *time.Timer

	// 运行控制
	running bool
	stopCh  chan struct{}
}

// NewRaftNode 创建新的 Raft 节点
func NewRaftNode(nodeID, masterAddr, selfAddr string) *RaftNode {
	rn := &RaftNode{
		nodeID:      nodeID,
		masterAddr:  masterAddr,
		selfAddr:    selfAddr,
		state:       Follower,
		currentTerm: 0,
		votedFor:    "",
		log:         make([]LogEntry, 0),
		commitIndex: 0,
		lastApplied: 0,
		running:     true,
		stopCh:      make(chan struct{}),
	}

	rn.resetElectionTimer()
	return rn
}

// resetElectionTimer 重置选举定时器
func (rn *RaftNode) resetElectionTimer() {
	if rn.electionTimer != nil {
		rn.electionTimer.Stop()
	}
	timeout := time.Duration(150+rand.Intn(150)) * time.Millisecond
	rn.electionTimer = time.NewTimer(timeout)
}

// RegisterToMaster 注册到 Master
func (rn *RaftNode) RegisterToMaster() error {
	req := map[string]string{
		"node_id": rn.nodeID,
		"address": rn.selfAddr,
	}

	jsonData, _ := json.Marshal(req)
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/register", rn.masterAddr),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("注册失败，状态码：%d", resp.StatusCode)
	}

	fmt.Printf("[Node %s] 已注册到 Master\n", rn.nodeID)
	return nil
}

// SendHeartbeat 发送心跳到 Master
func (rn *RaftNode) SendHeartbeat() error {
	req := map[string]string{
		"node_id": rn.nodeID,
	}

	jsonData, _ := json.Marshal(req)
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/heartbeat", rn.masterAddr),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// StartHeartbeatLoop 启动心跳循环
func (rn *RaftNode) StartHeartbeatLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for rn.running {
		select {
		case <-ticker.C:
			if err := rn.SendHeartbeat(); err != nil {
				fmt.Printf("[Node %s] 心跳失败：%v\n", rn.nodeID, err)
			}
		case <-rn.stopCh:
			return
		}
	}
}

// GetPeers 从 Master 获取其他节点列表
func (rn *RaftNode) GetPeers() ([]string, error) {
	resp, err := http.Get(fmt.Sprintf("http://%s/api/nodes", rn.masterAddr))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Nodes []struct {
			ID      string `json:"id"`
			Address string `json:"address"`
		} `json:"nodes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	peers := make([]string, 0)
	for _, node := range result.Nodes {
		if node.Address != rn.selfAddr {
			peers = append(peers, node.Address)
		}
	}

	return peers, nil
}

// StartElection 开始选举
func (rn *RaftNode) StartElection() {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.state = Candidate
	rn.currentTerm++
	rn.votedFor = rn.nodeID
	rn.votesReceived = 1

	fmt.Printf("[Node %s] 开始选举，Term %d\n", rn.nodeID, rn.currentTerm)

	// 获取节点列表
	peers, err := rn.GetPeers()
	if err != nil {
		fmt.Printf("[Node %s] 获取节点列表失败：%v\n", rn.nodeID, err)
		return
	}

	// 向其他节点请求投票
	for _, peerAddr := range peers {
		go rn.sendVoteRequest(peerAddr)
	}

	rn.resetElectionTimer()
}

// sendVoteRequest 发送投票请求
func (rn *RaftNode) sendVoteRequest(peerAddr string) {
	rn.mu.Lock()
	if rn.state != Candidate {
		rn.mu.Unlock()
		return
	}

	lastLogIndex := len(rn.log) - 1
	lastLogTerm := 0
	if lastLogIndex >= 0 {
		lastLogTerm = rn.log[lastLogIndex].Term
	}

	req := map[string]interface{}{
		"term":          rn.currentTerm,
		"candidate_id":  rn.nodeID,
		"last_log_index": lastLogIndex,
		"last_log_term":  lastLogTerm,
	}
	rn.mu.Unlock()

	jsonData, _ := json.Marshal(req)
	resp, err := http.Post(
		fmt.Sprintf("http://%s/api/vote", peerAddr),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var voteResp struct {
		Term        int  `json:"term"`
		VoteGranted bool `json:"vote_granted"`
	}
	json.NewDecoder(resp.Body).Decode(&voteResp)

	rn.mu.Lock()
	defer rn.mu.Unlock()

	if voteResp.Term > rn.currentTerm {
		rn.becomeFollower(voteResp.Term)
		return
	}

	if voteResp.VoteGranted && rn.state == Candidate && voteResp.Term == rn.currentTerm {
		rn.votesReceived++
		fmt.Printf("[Node %s] 收到投票，共 %d 票\n", rn.nodeID, rn.votesReceived)

		if rn.votesReceived > 2 { // 简单起见，假设 5 节点集群
			rn.becomeLeader()
		}
	}
}

// becomeLeader 成为 Leader
func (rn *RaftNode) becomeLeader() {
	rn.state = Leader
	fmt.Printf("[Node %s] 成为 Leader，Term %d\n", rn.nodeID, rn.currentTerm)

	// 向 Master 报告状态
	rn.reportStateToMaster()
}

// becomeFollower 成为 Follower
func (rn *RaftNode) becomeFollower(term int) {
	rn.state = Follower
	rn.currentTerm = term
	rn.votedFor = ""
	rn.resetElectionTimer()
	fmt.Printf("[Node %s] 成为 Follower，Term %d\n", rn.nodeID, term)
}

// reportStateToMaster 向 Master 报告状态
func (rn *RaftNode) reportStateToMaster() {
	req := map[string]interface{}{
		"key":   fmt.Sprintf("node_%s_state", rn.nodeID),
		"value": map[string]interface{}{
			"state": rn.state.String(),
			"term":  rn.currentTerm,
		},
	}

	jsonData, _ := json.Marshal(req)
	http.Post(
		fmt.Sprintf("http://%s/api/raft-state", rn.masterAddr),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
}

// StartElectionLoop 启动选举循环
func (rn *RaftNode) StartElectionLoop() {
	for rn.running {
		select {
		case <-rn.electionTimer.C:
			rn.StartElection()
		case <-rn.stopCh:
			if rn.electionTimer != nil {
				rn.electionTimer.Stop()
			}
			return
		}
	}
}

// Submit 提交命令
func (rn *RaftNode) Submit(command interface{}) bool {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state != Leader {
		return false
	}

	entry := LogEntry{
		Term:    rn.currentTerm,
		Command: command,
		Index:   len(rn.log),
	}
	rn.log = append(rn.log, entry)
	fmt.Printf("[Node %s] 提交命令到日志，索引 %d\n", rn.nodeID, entry.Index)

	return true
}

// GetState 获取状态
func (rn *RaftNode) GetState() (NodeState, int) {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	return rn.state, rn.currentTerm
}

// GetLog 获取日志
func (rn *RaftNode) GetLog() []LogEntry {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	return append([]LogEntry{}, rn.log...)
}

// Stop 停止节点
func (rn *RaftNode) Stop() {
	rn.running = false
	close(rn.stopCh)
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("用法：node <node_id> <master_addr> [self_addr]")
		fmt.Println("示例：node node1 localhost:8080 localhost:8081")
		os.Exit(1)
	}

	nodeID := os.Args[1]
	masterAddr := os.Args[2]
	selfAddr := "localhost:8081"
	if len(os.Args) > 3 {
		selfAddr = os.Args[3]
	}

	fmt.Printf("[Node %s] 启动，Master: %s, 本机：%s\n", nodeID, masterAddr, selfAddr)

	rn := NewRaftNode(nodeID, masterAddr, selfAddr)

	// 注册到 Master
	if err := rn.RegisterToMaster(); err != nil {
		fmt.Printf("[Node %s] 注册失败：%v\n", nodeID, err)
		os.Exit(1)
	}

	// 启动心跳循环
	go rn.StartHeartbeatLoop()

	// 启动选举循环
	go rn.StartElectionLoop()

	// 提取端口号
	port := "8081"
	parts := strings.Split(selfAddr, ":")
	if len(parts) > 1 {
		port = parts[1]
	}

	// 启动 HTTP 服务器
	httpServer := NewNodeHTTPServer(rn)
	go func() {
		if err := httpServer.Start(port); err != nil {
			fmt.Printf("[Node %s] HTTP 服务错误：%v\n", nodeID, err)
		}
	}()

	fmt.Printf("[Node %s] 运行中...\n", nodeID)

	// 保持运行
	select {}
}
