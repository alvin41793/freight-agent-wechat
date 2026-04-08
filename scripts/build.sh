#!/bin/bash
# ============================================================
# 编译脚本 - 指定目标操作系统和架构
# 用法: ./scripts/build.sh [os] [arch]
#
#   os   : linux | darwin | windows  (默认 linux)
#   arch : amd64 | arm64            (默认 amd64)
#
# 注意：运行模式（debug/pro）在启动时指定，与编译无关：
#   ./scripts/start.sh debug
#   ./scripts/start.sh pro
#
# 输出: bin/{os}_{arch}/freight-agent[.exe]
#
# 示例:
#   ./scripts/build.sh                    # linux/amd64（服务器默认）
#   ./scripts/build.sh darwin amd64      # macOS Intel
#   ./scripts/build.sh darwin arm64      # macOS Apple Silicon
#   ./scripts/build.sh linux arm64       # Linux ARM64
#   ./scripts/build.sh windows amd64     # Windows
# ============================================================

set -e

cd "$(dirname "$0")/.."

TARGET_OS=${1:-linux}
TARGET_ARCH=${2:-amd64}

# 校验 OS 参数
case "${TARGET_OS}" in
    linux|darwin|windows) ;;
    *)
        echo "[ERROR] 不支持的 OS: ${TARGET_OS}"
        echo "        合法值: linux | darwin | windows"
        echo "        用法: $0 [os] [arch]"
        echo ""
        echo "        注意: 运行模式（debug/pro）在启动时指定，不在编译时指定"
        echo "        启动: ./scripts/start.sh [debug|pro]"
        exit 1
        ;;
esac

# 校验 Arch 参数
case "${TARGET_ARCH}" in
    amd64|arm64) ;;
    *)
        echo "[ERROR] 不支持的架构: ${TARGET_ARCH}"
        echo "        合法值: amd64 | arm64"
        exit 1
        ;;
esac

BINARY="freight-agent"
if [ "$TARGET_OS" = "windows" ]; then
    BINARY="${BINARY}.exe"
fi

OUTPUT_DIR="bin/${TARGET_OS}_${TARGET_ARCH}"
OUTPUT="${OUTPUT_DIR}/${BINARY}"

mkdir -p "${OUTPUT_DIR}"

echo "==================================="
echo " Building freight-agent"
echo " OS   : ${TARGET_OS}"
echo " Arch : ${TARGET_ARCH}"
echo " Out  : ${OUTPUT}"
echo "==================================="

GOOS=${TARGET_OS} GOARCH=${TARGET_ARCH} go build \
    -trimpath \
    -ldflags "-s -w" \
    -o "${OUTPUT}" \
    ./cmd/server/

chmod +x "${OUTPUT}" 2>/dev/null || true

echo ""
echo "[BUILD] Done: ${OUTPUT}"
echo ""
echo "启动方式（以 linux 为例）:"
echo "  ${OUTPUT} --mode=debug   # debug 模式"
echo "  ${OUTPUT} --mode=pro     # 生产模式"
