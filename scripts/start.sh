#!/bin/bash
# ============================================================
# 货运 Agent 启动脚本
# 用法: ./scripts/start.sh [debug|pro]
#
#   debug - 开启 LLM 输入/输出文件日志，加载 configs/config.debug.yaml
#   pro   - 生产模式，加载 configs/config.pro.yaml（默认）
#
# 自动检测当前系统和架构：
#   - 若 bin/{os}_{arch}/freight-agent 存在，直接运行编译产物
#   - 否则回退到 go run（需已安装 Go）
# ============================================================

set -e

cd "$(dirname "$0")/.."

MODE=${1:-pro}

if [ "$MODE" != "debug" ] && [ "$MODE" != "pro" ]; then
    echo "[ERROR] 无效模式: $MODE，只支持 debug 或 pro"
    exit 1
fi

# 检查对应的配置文件是否存在
CONFIG_FILE="configs/config.${MODE}.yaml"
if [ ! -f "${CONFIG_FILE}" ]; then
    EXAMPLE_FILE="configs/config.${MODE}.yaml.example"
    echo "[ERROR] 配置文件 ${CONFIG_FILE} 不存在"
    if [ -f "${EXAMPLE_FILE}" ]; then
        echo "[HINT]  请复制模板并填入真实配置："
        echo "        cp ${EXAMPLE_FILE} ${CONFIG_FILE}"
    fi
    exit 1
fi

# 自动识别当前操作系统和 CPU 架构
RAW_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "${RAW_OS}" in
    darwin) OS="darwin" ;;
    linux)  OS="linux"  ;;
    *)      OS="linux"  ;;
esac

case $(uname -m) in
    x86_64)        ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             ARCH="amd64" ;;
esac

BINARY="bin/${OS}_${ARCH}/freight-agent"

if [ -f "${BINARY}" ]; then
    echo "[INFO] 使用编译产物: ${BINARY}"
    echo "[INFO] 以 ${MODE} 模式启动..."
    exec "${BINARY}" --mode=${MODE}
else
    # 回退到 go run
    if ! command -v go &>/dev/null; then
        echo "[ERROR] 未找到编译产物 ${BINARY}，且 Go 未安装"
        echo "[HINT]  请先编译: ./scripts/build.sh ${OS} ${ARCH}"
        exit 1
    fi
    echo "[INFO] 未找到编译产物，使用 go run"
    echo "[INFO] 以 ${MODE} 模式启动..."
    exec go run ./cmd/server/ --mode=${MODE}
fi
