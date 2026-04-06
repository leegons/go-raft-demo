package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// NodeHTTPServer 节点 HTTP 服务器
type NodeHTTPServer struct {
	node   *RaftNode
	mu     sync.Mutex
}

// NewNodeHTTPServer 创建节点 HTTP 服务器
func NewNodeHTTPServer(node *RaftNode) *NodeHTTPServer {
	return &NodeHTTPServer{
		node: node,
	}
}

// VoteRequest 投票请求
type VoteRequest struct {
	Term         int    `json:"term"`
	CandidateID  string `json:"candidate_id"`
	LastLogIndex int    `json:"last_log_index"`
	LastLogTerm  int    `json:"last_log_term"`
}

// VoteResponse 投票响应
type VoteResponse struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"vote_granted"`
}

// SubmitRequest 提交请求
type SubmitRequest struct {
	Command interface{} `json:"command"`
}

// voteHandler 处理投票请求
func (s *NodeHTTPServer) voteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(VoteResponse{VoteGranted: false})
		return
	}

	node := s.node
	node.mu.Lock()
	defer node.mu.Unlock()

	resp := VoteResponse{Term: node.currentTerm, VoteGranted: false}

	// 候选人的 Term 小于当前 Term，拒绝投票
	if req.Term < node.currentTerm {
		json.NewEncoder(w).Encode(resp)
		return
	}

	// 发现更大的 Term，退回 Follower 状态
	if req.Term > node.currentTerm {
		node.currentTerm = req.Term
		node.state = Follower
		node.votedFor = ""
	}
	resp.Term = node.currentTerm

	// 本 Term 已投票给其他候选人，拒绝
	if node.votedFor != "" && node.votedFor != req.CandidateID {
		json.NewEncoder(w).Encode(resp)
		return
	}

	// 检查候选人的日志是否至少和自己一样新（Raft 日志完整性检查）
	lastLogIndex := len(node.log) - 1
	lastLogTerm := 0
	if lastLogIndex >= 0 {
		lastLogTerm = node.log[lastLogIndex].Term
	}
	logUpToDate := req.LastLogTerm > lastLogTerm ||
		(req.LastLogTerm == lastLogTerm && req.LastLogIndex >= lastLogIndex)

	if !logUpToDate {
		json.NewEncoder(w).Encode(resp)
		return
	}

	// 授予投票
	node.votedFor = req.CandidateID
	node.resetElectionTimer()
	resp.VoteGranted = true

	fmt.Printf("[Node %s] 投票给 %s，Term %d\n", node.nodeID, req.CandidateID, req.Term)
	json.NewEncoder(w).Encode(resp)
}

// submitHandler 处理提交请求
func (s *NodeHTTPServer) submitHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.node.Submit(req.Command) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else {
		http.Error(w, "不是 Leader 节点", http.StatusServiceUnavailable)
	}
}

// logHandler 处理日志查询
func (s *NodeHTTPServer) logHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log := s.node.GetLog()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"log": log,
		"count": len(log),
	})
}

// Start 启动 HTTP 服务器
func (s *NodeHTTPServer) Start(addr string) error {
	http.HandleFunc("/api/vote", s.voteHandler)
	http.HandleFunc("/api/submit", s.submitHandler)
	http.HandleFunc("/api/log", s.logHandler)

	fmt.Printf("[Node %s] HTTP 服务启动：%s\n", s.node.nodeID, addr)
	return http.ListenAndServe(":"+addr, nil)
}
