package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// NodeInfo 节点信息
type NodeInfo struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	State     string    `json:"state"` // active, inactive
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// MasterService Master 服务
type MasterService struct {
	mu        sync.RWMutex
	nodes     map[string]*NodeInfo
	raftState map[string]interface{} // Raft 元信息
	config    map[string]interface{}
}

// NewMasterService 创建 Master 服务
func NewMasterService() *MasterService {
	return &MasterService{
		nodes:     make(map[string]*NodeInfo),
		raftState: make(map[string]interface{}),
		config:    make(map[string]interface{}),
	}
}

// RegisterNode 注册节点
func (m *MasterService) RegisterNode(nodeID, address string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nodes[nodeID] = &NodeInfo{
		ID:            nodeID,
		Address:       address,
		State:         "active",
		LastHeartbeat: time.Now(),
		Metadata:      make(map[string]interface{}),
	}

	fmt.Printf("[Master] 节点注册：%s (%s)\n", nodeID, address)
	return nil
}

// Heartbeat 节点心跳
func (m *MasterService) Heartbeat(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node, exists := m.nodes[nodeID]; exists {
		node.LastHeartbeat = time.Now()
		node.State = "active"
		return nil
	}
	return fmt.Errorf("节点不存在：%s", nodeID)
}

// GetActiveNodes 获取所有活跃节点
func (m *MasterService) GetActiveNodes() []*NodeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeNodes := make([]*NodeInfo, 0)
	for _, node := range m.nodes {
		if node.State == "active" {
			activeNodes = append(activeNodes, node)
		}
	}
	return activeNodes
}

// GetNode 获取单个节点信息
func (m *MasterService) GetNode(nodeID string) (*NodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if node, exists := m.nodes[nodeID]; exists {
		return node, nil
	}
	return nil, fmt.Errorf("节点不存在：%s", nodeID)
}

// SetRaftState 设置 Raft 状态
func (m *MasterService) SetRaftState(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.raftState[key] = value
}

// GetRaftState 获取 Raft 状态
func (m *MasterService) GetRaftState(key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, exists := m.raftState[key]
	return value, exists
}

// GetAllRaftState 获取所有 Raft 状态
func (m *MasterService) GetAllRaftState() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]interface{})
	for k, v := range m.raftState {
		result[k] = v
	}
	return result
}

// CheckNodeHealth 检查节点健康状态（后台协程）
func (m *MasterService) CheckNodeHealth() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for _, node := range m.nodes {
			if now.Sub(node.LastHeartbeat) > 15*time.Second {
				if node.State == "active" {
					fmt.Printf("[Master] 节点超时：%s\n", node.ID)
				}
				node.State = "inactive"
			}
		}
		m.mu.Unlock()
	}
}

// HTTP 处理器

func (m *MasterService) registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NodeID  string `json:"node_id"`
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := m.RegisterNode(req.NodeID, req.Address); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *MasterService) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NodeID string `json:"node_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := m.Heartbeat(req.NodeID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *MasterService) nodesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodes := m.GetActiveNodes()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	})
}

func (m *MasterService) raftStateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(m.GetAllRaftState())
	case http.MethodPost:
		var req struct {
			Key   string      `json:"key"`
			Value interface{} `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.SetRaftState(req.Key, req.Value)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Start 启动 Master 服务
func (m *MasterService) Start(port string) error {
	http.HandleFunc("/api/register", m.registerHandler)
	http.HandleFunc("/api/heartbeat", m.heartbeatHandler)
	http.HandleFunc("/api/nodes", m.nodesHandler)
	http.HandleFunc("/api/raft-state", m.raftStateHandler)

	// 启动健康检查
	go m.CheckNodeHealth()

	fmt.Printf("[Master] 启动服务，监听端口 %s\n", port)
	return http.ListenAndServe(":"+port, nil)
}

func main() {
	master := NewMasterService()
	if err := master.Start("8080"); err != nil {
		fmt.Printf("[Master] 服务错误：%v\n", err)
	}
}
