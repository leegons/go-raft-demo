# Go Raft Demo - Proxy-Master-Node 架构

用 Go 语言实现的 Raft 分布式共识算法演示项目，采用 **Proxy-Master-Node** 多进程架构。

## 🏗️ 架构设计

```
┌─────────────┐
│   Proxy     │  ← 用户交互入口，转发命令
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Master    │  ← 元信息管理，节点注册，心跳监控
└──────┬──────┘
       │
       ├──────────┬──────────┬──────────┬──────────┐
       ▼          ▼          ▼          ▼          ▼
   ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐
   │Node 1│  │Node 2│  │Node 3│  │Node 4│  │Node 5│
   └──────┘  └──────┘  └──────┘  └──────┘  └──────┘
      Raft 节点集群（领导者选举、日志复制）
```

### 组件说明

| 组件 | 职责 | 端口 |
|------|------|------|
| **Master** | 管理元信息、节点注册、心跳监控、Raft 状态存储 | 8080 |
| **Proxy** | 接收用户命令、查询集群状态、转发请求到 Leader | 交互式 CLI |
| **Node** | Raft 节点实现、领导者选举、日志复制、处理投票 | 8081+ |

## 🚀 快速开始

### 环境要求

- Go 1.21 或更高版本

### 1. 启动 Master 服务

```bash
cd go-raft-demo
go run master/main.go
```

Master 会在 `localhost:8080` 启动 HTTP 服务。

### 2. 启动多个 Node 节点

打开多个终端，分别启动节点：

```bash
# 终端 1 - Node 1
go run node/main.go node1 localhost:8080 localhost:8081

# 终端 2 - Node 2
go run node/main.go node2 localhost:8080 localhost:8082

# 终端 3 - Node 3
go run node/main.go node3 localhost:8080 localhost:8083

# 终端 4 - Node 4
go run node/main.go node4 localhost:8080 localhost:8084

# 终端 5 - Node 5
go run node/main.go node5 localhost:8080 localhost:8085
```

每个节点会：
- 向 Master 注册自己
- 每 3 秒发送心跳
- 参与 Raft 选举

### 3. 通过 Proxy 与集群交互

```bash
go run proxy/main.go localhost:8080
```

### Proxy 命令

| 命令 | 说明 |
|------|------|
| `status` | 查看集群状态 |
| `nodes` | 查看节点列表 |
| `leader` | 查看当前 Leader |
| `submit <消息>` | 提交命令到集群 |
| `log` | 查看 Leader 日志 |
| `quit` | 退出程序 |

## 📖 使用示例

### 启动集群

```bash
# 终端 1 - Master
$ go run master/main.go
[Master] 启动服务，监听端口 8080

# 终端 2 - Node 1
$ go run node/main.go node1 localhost:8080 localhost:8081
[Node node1] 启动，Master: localhost:8080, 本机：localhost:8081
[Node node1] 已注册到 Master
[Node node1] 运行中...

# 终端 3 - Node 2
$ go run node/main.go node2 localhost:8080 localhost:8082
[Node node2] 启动，Master: localhost:8080, 本机：localhost:8082
[Node node2] 已注册到 Master
[Node node2] 运行中...

# 继续启动更多节点...
```

### 通过 Proxy 操作

```bash
# 终端 N - Proxy
$ go run proxy/main.go localhost:8080
=== Raft 集群代理 ===
Master: localhost:8080

等待集群初始化...
=== 命令帮助 ===
  status    - 查看集群状态
  nodes     - 查看节点列表
  leader    - 查看 Leader 节点
  submit <消息> - 提交命令到集群
  log       - 查看 Leader 日志
  quit      - 退出程序

proxy> status

=== 集群状态 ===
节点数：5

节点列表:
  - node1 (localhost:8081) [active]
  - node2 (localhost:8082) [active]
  - node3 (localhost:8083) [active]
  - node4 (localhost:8084) [active]
  - node5 (localhost:8085) [active]

proxy> leader

✅ Leader: node1

proxy> submit "Hello Raft!"

✅ 命令已提交到 Leader (node1)

proxy> log

=== Leader 日志 ===
索引 0, Term 1: Hello Raft!

proxy> quit
退出程序...
```

## 📁 项目结构

```
go-raft-demo/
├── master/
│   └── main.go       # Master 服务（元信息管理）
├── node/
│   ├── main.go       # Raft 节点主程序
│   └── http.go       # 节点 HTTP 服务器
├── proxy/
│   └── main.go       # 代理服务（用户交互）
├── go.mod            # Go 模块定义
├── README.md         # 说明文档
└── .gitignore        # Git 忽略文件
```

## 🔧 Master API

### 节点管理

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/register` | POST | 注册节点 |
| `/api/heartbeat` | POST | 节点心跳 |
| `/api/nodes` | GET | 获取活跃节点列表 |

### Raft 状态

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/raft-state` | GET/POST | 获取/设置 Raft 状态 |

### 请求示例

```bash
# 注册节点
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node1", "address": "localhost:8081"}'

# 发送心跳
curl -X POST http://localhost:8080/api/heartbeat \
  -H "Content-Type: application/json" \
  -d '{"node_id": "node1"}'

# 获取节点列表
curl http://localhost:8080/api/nodes

# 获取 Raft 状态
curl http://localhost:8080/api/raft-state
```

## 🎯 Node HTTP API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/vote` | POST | 请求投票 |
| `/api/submit` | POST | 提交命令（仅 Leader） |
| `/api/log` | GET | 获取日志 |

## 🔍 架构优势

### 1. 职责分离

- **Master** 专注元信息管理，不参与 Raft 选举
- **Node** 专注 Raft 共识，通过 Master 发现彼此
- **Proxy** 提供统一入口，对用户隐藏集群细节

### 2. 易于扩展

- 添加新节点只需向 Master 注册
- Proxy 可以部署多个，负载均衡
- Master 可以持久化状态，支持重启恢复

### 3. 便于调试

- 集中查看集群状态
- 独立的日志输出
- 可以单独重启某个组件

## 🧪 扩展建议

1. **Master 持久化** - 使用数据库存储元信息
2. **Proxy 负载均衡** - 支持多个 Proxy 实例
3. **Node 快照** - 实现日志压缩
4. **成员变更** - 支持动态添加/删除节点
5. **监控告警** - 集成 Prometheus/Grafana

## 📚 参考资料

- [Raft 论文](https://raft.github.io/raft.pdf)
- [Raft 官方网站](https://raft.github.io/)
- [MIT 6.824 分布式系统课程](https://pdos.csail.mit.edu/6.824/)

## 📝 License

MIT License
