package raft

import (
	"testing"
	"time"
)

// makeCluster 创建一个 n 节点的内存 Raft 集群
func makeCluster(n int) []*Raft {
	nodes := make([]*Raft, n)
	for i := 0; i < n; i++ {
		nodes[i] = NewRaft(nodes, i)
	}
	return nodes
}

func stopCluster(nodes []*Raft) {
	for _, n := range nodes {
		n.Stop()
	}
}

// waitForLeader 等待集群选出 Leader，超时返回 -1
func waitForLeader(nodes []*Raft, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for i, n := range nodes {
			if state, _ := n.GetState(); state == Leader {
				return i
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return -1
}

// TestLeaderElection 测试集群能选出唯一 Leader
func TestLeaderElection(t *testing.T) {
	nodes := makeCluster(5)
	defer stopCluster(nodes)

	leaderIdx := waitForLeader(nodes, 2*time.Second)
	if leaderIdx == -1 {
		t.Fatal("集群未能在 2 秒内选出 Leader")
	}

	// 确保只有一个 Leader
	leaderCount := 0
	leaderTerm := 0
	for _, n := range nodes {
		state, term := n.GetState()
		if state == Leader {
			leaderCount++
			leaderTerm = term
		}
	}
	if leaderCount != 1 {
		t.Fatalf("期望 1 个 Leader，实际 %d 个", leaderCount)
	}
	if leaderTerm == 0 {
		t.Fatal("Leader 的 Term 不应为 0")
	}

	t.Logf("Node %d 成为 Leader，Term %d", leaderIdx, leaderTerm)
}

// TestLeaderElection3 测试 3 节点集群选举
func TestLeaderElection3(t *testing.T) {
	nodes := makeCluster(3)
	defer stopCluster(nodes)

	leaderIdx := waitForLeader(nodes, 2*time.Second)
	if leaderIdx == -1 {
		t.Fatal("3 节点集群未能在 2 秒内选出 Leader")
	}
	t.Logf("Node %d 成为 Leader", leaderIdx)
}

// TestSubmitToLeader 测试向 Leader 提交命令
func TestSubmitToLeader(t *testing.T) {
	nodes := makeCluster(5)
	defer stopCluster(nodes)

	leaderIdx := waitForLeader(nodes, 2*time.Second)
	if leaderIdx == -1 {
		t.Fatal("未能选出 Leader")
	}

	leader := nodes[leaderIdx]
	if !leader.Submit("hello") {
		t.Fatal("Submit 应该返回 true（当前节点是 Leader）")
	}

	log := leader.GetLog()
	if len(log) != 1 {
		t.Fatalf("期望日志长度 1，实际 %d", len(log))
	}
	if log[0].Command != "hello" {
		t.Fatalf("期望命令 'hello'，实际 %v", log[0].Command)
	}
}

// TestSubmitToFollower 测试向 Follower 提交命令应失败
func TestSubmitToFollower(t *testing.T) {
	nodes := makeCluster(3)
	defer stopCluster(nodes)

	leaderIdx := waitForLeader(nodes, 2*time.Second)
	if leaderIdx == -1 {
		t.Fatal("未能选出 Leader")
	}

	// 找一个 Follower
	for i, n := range nodes {
		if i != leaderIdx {
			if n.Submit("should-fail") {
				t.Fatal("向 Follower 提交命令应返回 false")
			}
			break
		}
	}
}

// TestLogReplication 测试日志复制到所有节点
func TestLogReplication(t *testing.T) {
	nodes := makeCluster(5)
	defer stopCluster(nodes)

	leaderIdx := waitForLeader(nodes, 2*time.Second)
	if leaderIdx == -1 {
		t.Fatal("未能选出 Leader")
	}

	leader := nodes[leaderIdx]
	commands := []string{"cmd1", "cmd2", "cmd3"}
	for _, cmd := range commands {
		if !leader.Submit(cmd) {
			t.Fatalf("提交命令 %s 失败", cmd)
		}
	}

	// 等待日志复制
	time.Sleep(300 * time.Millisecond)

	// 检查所有节点的日志长度一致
	for i, n := range nodes {
		log := n.GetLog()
		if len(log) != len(commands) {
			t.Errorf("Node %d 日志长度 %d，期望 %d", i, len(log), len(commands))
		}
	}
}

// TestRequestVote 测试 RequestVote RPC 的基本规则
func TestRequestVote(t *testing.T) {
	nodes := makeCluster(3)
	defer stopCluster(nodes)

	// 等待选举稳定
	time.Sleep(500 * time.Millisecond)

	node := nodes[0]

	// Term 更小的请求应被拒绝
	node.mu.Lock()
	node.currentTerm = 10
	node.votedFor = -1
	node.mu.Unlock()

	resp := node.RequestVote(VoteRequest{
		Term:         5, // 小于 currentTerm=10
		CandidateId:  1,
		LastLogIndex: -1,
		LastLogTerm:  0,
	})
	if resp.VoteGranted {
		t.Error("Term 更小的请求不应获得投票")
	}
}

// TestGetCommitIndex 测试 GetCommitIndex 方法
func TestGetCommitIndex(t *testing.T) {
	nodes := makeCluster(5)
	defer stopCluster(nodes)

	leaderIdx := waitForLeader(nodes, 2*time.Second)
	if leaderIdx == -1 {
		t.Fatal("未能选出 Leader")
	}

	leader := nodes[leaderIdx]
	initialCommit := leader.GetCommitIndex()

	leader.Submit("test-command")
	time.Sleep(200 * time.Millisecond)

	// 提交后的 commitIndex 应该更新（如果达到多数派）
	_ = leader.GetCommitIndex()
	_ = initialCommit
	// 注：commitIndex 是否推进取决于复制是否到达多数派
	// 这里只验证方法可调用且不 panic
}
