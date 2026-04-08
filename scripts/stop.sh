#!/bin/bash

# 货运 Agent - 停止脚本

cd "$(dirname "$0")/.."

# 停止 go run 进程
pids=$(pgrep -f "go run ./cmd/server/" 2>/dev/null || true)
if [ -n "$pids" ]; then
    echo "[INFO] 停止 go run 进程..."
    kill $pids 2>/dev/null || true
fi

# 停止编译后的二进制
pids=$(pgrep -f "freight-agent" 2>/dev/null || true)
if [ -n "$pids" ]; then
    echo "[INFO] 停止 freight-agent 进程..."
    kill $pids 2>/dev/null || true
fi

sleep 1
echo "[INFO] 服务已停止"
