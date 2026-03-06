#!/bin/bash

# Raft 集群停止脚本
# 用法：./stop-cluster.sh

echo "=== 停止 Raft 集群 ==="

# 查找并停止相关进程
PIDS=$(pgrep -f "go run (master|node|proxy)/main.go")

if [ -z "$PIDS" ]; then
    echo "未找到运行中的集群进程"
    exit 0
fi

echo "找到以下进程:"
echo "$PIDS"
echo ""

echo "停止进程..."
kill $PIDS 2>/dev/null

# 等待进程退出
sleep 2

# 检查是否还有残留进程
REMAINING=$(pgrep -f "go run (master|node|proxy)/main.go")
if [ -n "$REMAINING" ]; then
    echo "强制停止残留进程..."
    kill -9 $REMAINING 2>/dev/null
fi

echo "✅ 集群已停止"
