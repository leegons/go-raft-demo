#!/bin/bash

# Raft 集群快速启动脚本
# 用法：./start-cluster.sh

MASTER_PORT=8080
NODE_PORTS=(8081 8082 8083 8084 8085)
NUM_NODES=${#NODE_PORTS[@]}

echo "=== Raft 集群启动脚本 ==="
echo "Master 端口：$MASTER_PORT"
echo "节点端口：${NODE_PORTS[*]}"
echo "节点数量：$NUM_NODES"
echo ""

# 清理之前的进程
echo "清理之前的进程..."
pkill -f "go run master/main.go" 2>/dev/null
pkill -f "go run node/main.go" 2>/dev/null
pkill -f "go run proxy/main.go" 2>/dev/null
sleep 1

# 启动 Master
echo "🚀 启动 Master..."
go run master/main.go &
MASTER_PID=$!
sleep 2

# 启动 Nodes
echo "🚀 启动 $NUM_NODES 个节点..."
NODE_PIDS=()
for i in "${!NODE_PORTS[@]}"; do
    NODE_ID="node$((i+1))"
    NODE_PORT=${NODE_PORTS[$i]}
    go run node/main.go $NODE_ID localhost:$MASTER_PORT localhost:$NODE_PORT &
    NODE_PIDS+=($!)
    echo "  - $NODE_ID (端口 $NODE_PORT)"
    sleep 0.5
done

echo ""
echo "✅ 集群启动完成！"
echo ""
echo "进程信息:"
echo "  Master PID: $MASTER_PID"
echo "  Node PIDs: ${NODE_PIDS[*]}"
echo ""
echo "使用 Proxy 与集群交互:"
echo "  go run proxy/main.go localhost:$MASTER_PORT"
echo ""
echo "停止集群:"
echo "  ./stop-cluster.sh"
echo "  或运行：kill $MASTER_PID ${NODE_PIDS[*]}"
echo ""

# 等待
wait
