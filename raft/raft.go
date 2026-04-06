package raft

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// State 表示 Raft 节点的状态
type State int

const (
	Follower State = iota
	Candidate
	Leader
)

func (s State) String() string {
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

// LogEntry 表示日志条目
type LogEntry struct {
	Term    int
	Command interface{}
	Index   int
}

// VoteRequest 表示投票请求
type VoteRequest struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// VoteResponse 表示投票响应
type VoteResponse struct {
	Term        int
	VoteGranted bool
}

// AppendEntriesRequest 表示日志追加请求
type AppendEntriesRequest struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

// AppendEntriesResponse 表示日志追加响应
type AppendEntriesResponse struct {
	Term    int
	Success bool
}

// Raft Raft 节点结构
type Raft struct {
	mu sync.Mutex

	// 持久化状态
	currentTerm int
	votedFor    int
	log         []LogEntry

	// 易失性状态
	state      State
	commitIndex int
	lastApplied int

	// Leader 状态（易失性）
	nextIndex  []int
	matchIndex []int

	// 配置
	peers     []*Raft
	me        int
	electionTimeout time.Duration
	heartbeatTimeout time.Duration

	// 选举相关
	votesReceived int
	electionTimer *time.Timer

	// 应用层通信
	applyCh chan interface{}

	// 运行控制
	running bool
	stopCh  chan struct{}
}

// NewRaft 创建新的 Raft 节点
func NewRaft(peers []*Raft, me int) *Raft {
	rf := &Raft{
		peers:            peers,
		me:               me,
		state:            Follower,
		currentTerm:      0,
		votedFor:         -1,
		log:              make([]LogEntry, 0),
		commitIndex:      0,
		lastApplied:      0,
		electionTimeout:  time.Duration(150+rand.Intn(150)) * time.Millisecond,
		heartbeatTimeout: 50 * time.Millisecond,
		applyCh:          make(chan interface{}, 100),
		stopCh:           make(chan struct{}),
		running:          true,
	}

	// 初始化 Leader 状态
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))

	// 启动选举定时器
	rf.resetElectionTimer()

	// 启动后台协程
	go rf.electionLoop()
	go rf.applyLoop()

	return rf
}

// resetElectionTimer 重置选举定时器
func (rf *Raft) resetElectionTimer() {
	if rf.electionTimer != nil {
		rf.electionTimer.Stop()
	}
	timeout := time.Duration(150+rand.Intn(150)) * time.Millisecond
	rf.electionTimer = time.NewTimer(timeout)
}

// electionLoop 选举循环（Follower 和 Candidate）
func (rf *Raft) electionLoop() {
	for rf.running {
		select {
		case <-rf.electionTimer.C:
			rf.startElection()
		case <-rf.stopCh:
			return
		}
	}
}

// startElection 开始新的选举
func (rf *Raft) startElection() {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// 转为 Candidate 状态
	rf.state = Candidate
	rf.currentTerm++
	rf.votedFor = rf.me
	rf.votesReceived = 1 // 给自己投票

	fmt.Printf("[Node %d] 开始选举，Term %d\n", rf.me, rf.currentTerm)

	// 向其他节点请求投票
	for peerId := range rf.peers {
		if peerId != rf.me {
			go rf.sendVoteRequest(peerId)
		}
	}

	// 重置选举定时器
	rf.resetElectionTimer()
}

// sendVoteRequest 发送投票请求
func (rf *Raft) sendVoteRequest(peerId int) {
	rf.mu.Lock()
	if rf.state != Candidate {
		rf.mu.Unlock()
		return
	}

	lastLogIndex := len(rf.log) - 1
	lastLogTerm := 0
	if lastLogIndex >= 0 {
		lastLogTerm = rf.log[lastLogIndex].Term
	}

	req := VoteRequest{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}
	rf.mu.Unlock()

	// 模拟 RPC 调用
	resp := rf.peers[peerId].RequestVote(req)

	rf.mu.Lock()
	defer rf.mu.Unlock()

	// 如果响应中的 Term 更大，转为 Follower
	if resp.Term > rf.currentTerm {
		rf.becomeFollower(resp.Term)
		return
	}

	// 如果获得投票，计数加一
	if resp.VoteGranted && rf.state == Candidate && resp.Term == rf.currentTerm {
		rf.votesReceived++
		fmt.Printf("[Node %d] 收到来自 Node %d 的投票，共 %d 票\n", rf.me, peerId, rf.votesReceived)

		// 如果获得多数票，成为 Leader
		if rf.votesReceived > len(rf.peers)/2 {
			rf.becomeLeader()
		}
	}
}

// RequestVote 处理投票请求（RPC）
func (rf *Raft) RequestVote(req VoteRequest) VoteResponse {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// 如果请求的 Term 小于当前 Term，拒绝（用当前 Term 回复）
	if req.Term < rf.currentTerm {
		return VoteResponse{Term: rf.currentTerm, VoteGranted: false}
	}

	// 如果请求的 Term 大于当前 Term，转为 Follower（更新 currentTerm）
	if req.Term > rf.currentTerm {
		rf.becomeFollower(req.Term)
	}

	// 此时 currentTerm 已与 req.Term 对齐
	resp := VoteResponse{Term: rf.currentTerm, VoteGranted: false}

	// 检查是否已投票给其他候选人
	if rf.votedFor != -1 && rf.votedFor != req.CandidateId {
		return resp
	}

	// 检查候选人的日志是否至少和自己一样新
	lastLogIndex := len(rf.log) - 1
	lastLogTerm := 0
	if lastLogIndex >= 0 {
		lastLogTerm = rf.log[lastLogIndex].Term
	}

	if req.LastLogTerm < lastLogTerm {
		return resp
	}
	if req.LastLogTerm == lastLogTerm && req.LastLogIndex < lastLogIndex {
		return resp
	}

	// 投票给候选人
	rf.votedFor = req.CandidateId
	resp.VoteGranted = true

	fmt.Printf("[Node %d] 投票给 Node %d，Term %d\n", rf.me, req.CandidateId, req.Term)

	return resp
}

// becomeLeader 成为 Leader
func (rf *Raft) becomeLeader() {
	rf.state = Leader
	fmt.Printf("[Node %d] 成为 Leader，Term %d\n", rf.me, rf.currentTerm)

	// 初始化 nextIndex 和 matchIndex
	for i := range rf.peers {
		rf.nextIndex[i] = len(rf.log)
		rf.matchIndex[i] = -1
	}

	// 启动心跳循环（独立 goroutine，避免 time.AfterFunc 定时器累积）
	go rf.heartbeatLoop()
}

// becomeFollower 成为 Follower
func (rf *Raft) becomeFollower(term int) {
	rf.state = Follower
	rf.currentTerm = term
	rf.votedFor = -1
	rf.resetElectionTimer()
	fmt.Printf("[Node %d] 成为 Follower，Term %d\n", rf.me, term)
}

// heartbeatLoop 以固定间隔向所有 Follower 发送心跳，直到不再是 Leader
func (rf *Raft) heartbeatLoop() {
	ticker := time.NewTicker(rf.heartbeatTimeout)
	defer ticker.Stop()

	// 立即发送一次心跳
	rf.sendHeartbeatsOnce()

	for {
		select {
		case <-ticker.C:
			rf.mu.Lock()
			isLeader := rf.state == Leader && rf.running
			rf.mu.Unlock()
			if !isLeader {
				return
			}
			rf.sendHeartbeatsOnce()
		case <-rf.stopCh:
			return
		}
	}
}

// sendHeartbeatsOnce 向所有 Follower 发送一次 AppendEntries（心跳或日志复制）
func (rf *Raft) sendHeartbeatsOnce() {
	rf.mu.Lock()
	if rf.state != Leader || !rf.running {
		rf.mu.Unlock()
		return
	}
	peers := rf.peers
	me := rf.me
	rf.mu.Unlock()

	for peerId := range peers {
		if peerId != me {
			go rf.sendAppendEntries(peerId)
		}
	}
}

// sendAppendEntries 发送日志追加请求
func (rf *Raft) sendAppendEntries(peerId int) {
	rf.mu.Lock()
	if rf.state != Leader {
		rf.mu.Unlock()
		return
	}

	prevLogIndex := rf.nextIndex[peerId] - 1
	prevLogTerm := 0
	if prevLogIndex >= 0 && prevLogIndex < len(rf.log) {
		prevLogTerm = rf.log[prevLogIndex].Term
	}

	// 获取要发送的日志条目
	entries := make([]LogEntry, 0)
	if rf.nextIndex[peerId] < len(rf.log) {
		entries = rf.log[rf.nextIndex[peerId]:]
	}

	req := AppendEntriesRequest{
		Term:         rf.currentTerm,
		LeaderId:     rf.me,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: rf.commitIndex,
	}
	rf.mu.Unlock()

	// 模拟 RPC 调用
	resp := rf.peers[peerId].AppendEntries(req)

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.state != Leader {
		return
	}

	// 如果响应中的 Term 更大，转为 Follower
	if resp.Term > rf.currentTerm {
		rf.becomeFollower(resp.Term)
		return
	}

	// 更新 nextIndex 和 matchIndex
	if resp.Success {
		newMatchIndex := prevLogIndex + len(entries)
		if newMatchIndex > rf.matchIndex[peerId] {
			rf.matchIndex[peerId] = newMatchIndex
		}
		// nextIndex 指向下一条待发送的日志（matchIndex 之后）
		if newMatchIndex+1 > rf.nextIndex[peerId] {
			rf.nextIndex[peerId] = newMatchIndex + 1
		}

		// 检查是否可以提交日志
		rf.updateCommitIndex()
	} else {
		// 发送失败，递减 nextIndex 重试
		if rf.nextIndex[peerId] > 0 {
			rf.nextIndex[peerId]--
		}
	}
}

// updateCommitIndex 更新提交索引
func (rf *Raft) updateCommitIndex() {
	for n := rf.commitIndex + 1; n < len(rf.log); n++ {
		// 只提交当前 Term 的日志（Raft 安全性要求）
		if rf.log[n].Term != rf.currentTerm {
			continue
		}
		// Leader 自身算 1 票，统计已复制到多数节点的日志
		count := 1
		for i := range rf.peers {
			if i != rf.me && rf.matchIndex[i] >= n {
				count++
			}
		}
		if count > len(rf.peers)/2 {
			rf.commitIndex = n
			fmt.Printf("[Node %d] 提交日志索引 %d\n", rf.me, n)
		}
	}
}

// AppendEntries 处理日志追加请求（RPC）
func (rf *Raft) AppendEntries(req AppendEntriesRequest) AppendEntriesResponse {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	resp := AppendEntriesResponse{Term: rf.currentTerm, Success: false}

	// 如果请求的 Term 小于当前 Term，拒绝
	if req.Term < rf.currentTerm {
		return resp
	}

	// 如果请求的 Term 大于等于当前 Term，转为 Follower（becomeFollower 内已重置选举定时器）
	if req.Term >= rf.currentTerm {
		rf.becomeFollower(req.Term)
	}

	// 检查 PrevLogIndex 和 PrevLogTerm
	if req.PrevLogIndex >= len(rf.log) {
		return resp
	}

	if req.PrevLogIndex >= 0 {
		if rf.log[req.PrevLogIndex].Term != req.PrevLogTerm {
			// 日志不匹配，删除冲突的日志及之后的所有日志
			rf.log = rf.log[:req.PrevLogIndex]
			return resp
		}
	}

	// 追加日志
	if len(req.Entries) > 0 {
		insertIndex := req.PrevLogIndex + 1
		if insertIndex < len(rf.log) {
			rf.log = append(rf.log[:insertIndex], req.Entries...)
		} else {
			rf.log = append(rf.log, req.Entries...)
		}
	}

	// 更新 commitIndex
	if req.LeaderCommit > rf.commitIndex {
		rf.commitIndex = min(req.LeaderCommit, len(rf.log)-1)
	}

	resp.Success = true
	return resp
}

// Submit 提交命令到 Raft 日志
func (rf *Raft) Submit(command interface{}) bool {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.state != Leader {
		return false
	}

	entry := LogEntry{
		Term:    rf.currentTerm,
		Command: command,
		Index:   len(rf.log),
	}
	rf.log = append(rf.log, entry)
	fmt.Printf("[Node %d] 提交命令到日志，索引 %d\n", rf.me, entry.Index)

	return true
}

// applyLoop 应用日志到状态机
func (rf *Raft) applyLoop() {
	for rf.running {
		rf.mu.Lock()
		if rf.lastApplied < rf.commitIndex {
			rf.lastApplied++
			command := rf.log[rf.lastApplied].Command
			rf.mu.Unlock()

			// 应用到状态机（这里只是打印）
			fmt.Printf("[Node %d] 应用命令：%v\n", rf.me, command)
			rf.applyCh <- command
		} else {
			rf.mu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// GetState 获取节点状态
func (rf *Raft) GetState() (State, int) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.state, rf.currentTerm
}

// GetCommitIndex 获取已提交的最高日志索引
func (rf *Raft) GetCommitIndex() int {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.commitIndex
}

// GetLog 获取日志
func (rf *Raft) GetLog() []LogEntry {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return append([]LogEntry{}, rf.log...)
}

// Stop 停止节点
func (rf *Raft) Stop() {
	rf.running = false
	close(rf.stopCh)
	if rf.electionTimer != nil {
		rf.electionTimer.Stop()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
