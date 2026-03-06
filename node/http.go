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

	s.mu.Lock()
	defer s.mu.Unlock()

	// 简化处理：总是投票给第一个请求者（实际应该检查日志新旧）
	resp := VoteResponse{
		Term:        req.Term,
		VoteGranted: true,
	}

	fmt.Printf("[Node %s] 收到投票请求，Term %d，投票：%v\n",
		s.node.nodeID, req.Term, resp.VoteGranted)

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
