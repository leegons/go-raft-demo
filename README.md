# Go Raft Demo

用 Go 语言实现的 Raft 分布式共识算法演示项目。

## 📚 关于 Raft

Raft 是一种易于理解的分布式共识算法，用于管理复制日志。它的主要特点：

- **易于理解**：相比 Paxos，Raft 更直观
- **领导者选举**：集群自动选举 Leader
- **日志复制**：Leader 负责将日志复制到 Follower
- **安全性**：保证日志一致性和状态机安全

### Raft 节点状态

1. **Follower** - 被动响应 Leader 和 Candidate 的请求
2. **Candidate** - 发起选举，争取成为 Leader
3. **Leader** - 处理客户端请求，复制日志到 Follower

## 🚀 快速开始

### 环境要求

- Go 1.21 或更高版本

### 安装依赖

```bash
cd go-raft-demo
go mod tidy
```

### 运行 Demo

```bash
go run main.go
```

### 交互式命令

启动后会进入交互模式，支持以下命令：

| 命令 | 说明 |
|------|------|
| `submit <消息>` | 提交命令到 Raft 集群 |
| `status` | 查看所有节点状态 |
| `log` | 查看 Leader 日志 |
| `quit` | 退出程序 |

### 示例会话

```bash
$ go run main.go
=== Raft 分布式共识算法 Demo ===
创建一个包含 5 个节点的 Raft 集群

✅ 已创建 5 个 Raft 节点
等待选举产生 Leader...

=== 节点状态 ===
Node 0: Follower (Term: 1)
Node 1: Leader (Term: 1)
Node 2: Follower (Term: 1)
Node 3: Follower (Term: 1)
Node 4: Follower (Term: 1)

✅ Node 1 成为 Leader

=== 命令帮助 ===
  submit <消息>  - 提交命令到 Raft 集群
  status         - 查看所有节点状态
  log            - 查看 Leader 日志
  quit           - 退出程序

raft> submit "Hello Raft!"
✅ 命令已提交到 Node 1 (Leader)

raft> log

=== Leader 日志 ===
索引 0, Term 1: Hello Raft!

raft> status

=== 节点状态 ===
Node 0: Follower   Term: 1, 日志条目：1, 已提交：1
Node 1: Leader     Term: 1, 日志条目：1, 已提交：1
Node 2: Follower   Term: 1, 日志条目：1, 已提交：1
Node 3: Follower   Term: 1, 日志条目：1, 已提交：1
Node 4: Follower   Term: 1, 日志条目：1, 已提交：1

raft> quit
退出程序...
停止所有节点...
```

## 📁 项目结构

```
go-raft-demo/
├── main.go          # 演示程序入口
├── raft/
│   └── raft.go      # Raft 核心实现
├── go.mod           # Go 模块定义
└── README.md        # 说明文档
```

## 🔧 核心功能

### 领导者选举 (Leader Election)

- Follower 在选举超时时转为 Candidate
- Candidate 请求其他节点投票
- 获得多数票后成为 Leader
- 随机选举超时避免选票分裂

### 日志复制 (Log Replication)

- Leader 接收客户端命令并追加到日志
- 发送 AppendEntries RPC 到 Follower
- 当日志被多数节点复制后提交
- 定期发送心跳维持领导地位

### 安全性保证

- 选举限制：只投票给日志更新的候选人
- Leader 完整性：Leader 包含所有已提交的日志
- 状态机安全：所有节点按相同顺序应用日志

## 📖 代码结构

### Raft 结构体

```go
type Raft struct {
    // 持久化状态
    currentTerm int      // 当前任期
    votedFor    int      // 投票给谁
    log         []LogEntry  // 日志
    
    // 易失性状态
    state         State  // 节点状态
    commitIndex   int    // 已提交索引
    lastApplied   int    // 已应用索引
    
    // Leader 状态
    nextIndex   []int    // 下一个发送索引
    matchIndex  []int    // 已匹配索引
}
```

### RPC 接口

| RPC | 用途 |
|-----|------|
| `RequestVote` | 请求投票 |
| `AppendEntries` | 复制日志/心跳 |

## 🧪 扩展建议

这个项目是教学演示，可以扩展：

1. **持久化** - 将状态保存到磁盘
2. **网络层** - 使用真实网络通信
3. **成员变更** - 支持动态添加/删除节点
4. **快照** - 实现日志压缩
5. **测试** - 添加单元测试和压力测试

## 📚 参考资料

- [Raft 论文](https://raft.github.io/raft.pdf)
- [Raft 官方网站](https://raft.github.io/)
- [MIT 6.824 分布式系统课程](https://pdos.csail.mit.edu/6.824/)

## 📝 License

MIT License
