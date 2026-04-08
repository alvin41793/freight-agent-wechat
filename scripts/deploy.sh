#!/bin/bash
# ============================================================
# 一键生产部署脚本
# 用法: ./scripts/deploy.sh
#
# 流程：
#   1. 本地交叉编译 linux/amd64 二进制
#   2. rsync 推送到服务器（binary / configs / migrations / docker-compose）
#   3. 服务器端启动 MySQL + Redis（docker-compose infra）
#   4. 注册 / 重启 freight-agent systemd 服务
#
# 前置要求：
#   本地: rsync, ssh 密钥已添加到服务器（ssh-copy-id root@123.56.177.18）
#   服务器: Docker + docker-compose 已安装
# ============================================================

set -euo pipefail

# ---------- 配置 ----------
REMOTE_HOST="root@123.56.177.18"
REMOTE_DIR="/opt/freight-agent"
APP_NAME="freight-agent"
SERVICE_NAME="freight-agent"
BINARY_LOCAL="bin/linux_amd64/${APP_NAME}"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

step() { echo -e "\n${BLUE}[$(date +%H:%M:%S)]${NC} $*"; }
ok()   { echo -e "  ${GREEN}✓${NC} $*"; }
warn() { echo -e "  ${YELLOW}⚠${NC} $*"; }
err()  { echo -e "  ${RED}✗${NC} $*"; exit 1; }

cd "$(dirname "$0")/.."

echo -e "${BLUE}"
echo "  ╔══════════════════════════════════════╗"
echo "  ║   freight-agent 生产部署             ║"
echo "  ║   目标: ${REMOTE_HOST}  ║"
echo "  ╚══════════════════════════════════════╝"
echo -e "${NC}"

# ---------- 前置检查 ----------
step "前置检查..."
command -v rsync &>/dev/null || err "rsync 未安装，请: brew install rsync"
command -v ssh   &>/dev/null || err "ssh 未安装"
command -v go    &>/dev/null || err "go 未安装"

# 检查 config.pro.yaml 是否已填写真实值
if grep -q "your_db_host\|your_api_key\|your_redis_password" configs/config.pro.yaml 2>/dev/null; then
    warn "configs/config.pro.yaml 仍包含占位符，请先填写真实配置："
    warn "  vim configs/config.pro.yaml"
    read -rp "  仍要继续部署？[y/N] " confirm
    [[ "$confirm" =~ ^[Yy]$ ]] || exit 1
fi

# SSH 连通性测试
ssh -o ConnectTimeout=5 -o BatchMode=yes "${REMOTE_HOST}" "echo ok" &>/dev/null \
    || err "SSH 连接失败，请确认密钥已配置: ssh-copy-id ${REMOTE_HOST}"
ok "SSH 连通"

# ---------- 1. 本地编译 ----------
step "1/4  本地编译 linux/amd64..."
mkdir -p "$(dirname "${BINARY_LOCAL}")"
GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags "-s -w" \
    -o "${BINARY_LOCAL}" \
    ./cmd/server/
ok "编译完成: ${BINARY_LOCAL} ($(du -sh "${BINARY_LOCAL}" | cut -f1))"

# ---------- 2. rsync 推送 ----------
step "2/4  推送文件到服务器..."

# 创建远端目录结构
ssh "${REMOTE_HOST}" "mkdir -p ${REMOTE_DIR}/{bin,configs,migrations,logs}"

# 二进制
rsync -az --progress "${BINARY_LOCAL}" "${REMOTE_HOST}:${REMOTE_DIR}/bin/"
ok "二进制已上传"

# 生产配置
rsync -az configs/config.pro.yaml "${REMOTE_HOST}:${REMOTE_DIR}/configs/"
ok "configs/config.pro.yaml 已上传"

# 数据库迁移脚本
rsync -az migrations/ "${REMOTE_HOST}:${REMOTE_DIR}/migrations/"
ok "migrations/ 已上传"

# docker-compose（仅用于基础设施 mysql+redis）
rsync -az docker-compose.yml "${REMOTE_HOST}:${REMOTE_DIR}/"
ok "docker-compose.yml 已上传"

# 服务管理脚本
rsync -az scripts/service.sh "${REMOTE_HOST}:${REMOTE_DIR}/service.sh"
ssh "${REMOTE_HOST}" "chmod +x ${REMOTE_DIR}/service.sh"
ok "service.sh 已上传"

# ---------- 3. 服务器端：端口检查 + 启动基础设施 ----------
step "3/4  服务器端：端口检查 + 启动 MySQL + Redis..."
ssh "${REMOTE_HOST}" bash <<'REMOTE_INFRA'
set -e
cd /opt/freight-agent

# 先停止旧服务，避免端口检查误报
if systemctl is-active --quiet freight-agent 2>/dev/null; then
    systemctl stop freight-agent
    echo "  旧服务已停止"
fi

# ----- 端口占用检查 -----
check_port() {
    local port=$1
    local name=$2
    local pid
    # 使用 grep -oE 替代 3参数 match()，兼容旧版 awk/mawk
    pid=$(ss -tlnp "sport = :${port}" 2>/dev/null | grep -oE 'pid=[0-9]+' | grep -oE '[0-9]+' | head -1)
    if [ -n "$pid" ]; then
        local pname
        pname=$(ps -p "$pid" -o comm= 2>/dev/null || echo "unknown")
        echo "  [warn] 端口 ${port}(${name}) 已被进程占用: PID=${pid} (${pname})"
        return 1
    fi
    return 0
}

PORT_CONFLICT=0
# MySQL: 如果已是 docker-compose 起的 mysql，就是允许的
# 所以只针对非 docker mysql 的占用倰报错
for PORT_CHECK in "8080:freight-agent"; do
    PORT=$(echo $PORT_CHECK | cut -d: -f1)
    NAME=$(echo $PORT_CHECK | cut -d: -f2)
    if ! check_port $PORT $NAME; then
        PORT_CONFLICT=1
    fi
done

if [ $PORT_CONFLICT -eq 1 ]; then
    echo "  [error] 存在端口冲突，请手动处理后重试"
    exit 1
fi
echo "  端口检查通过 (8080 可用)"

# MySQL 密码已确定在 docker-compose.yml 中（MYSQL_ROOT_PASSWORD=123456），不再依赖 .env

# 拉取基础镜像（首次会下载）
docker-compose pull mysql redis 2>/dev/null || true

# 启动 MySQL + Redis（跳过 app service）
docker-compose up -d mysql redis

# 等待 MySQL 完全就绪（mysqladmin ping 通过后进一步确认可查询）
echo -n "  等待 MySQL 就绪"
for i in $(seq 1 30); do
    if docker-compose exec -T mysql mysqladmin ping -h localhost -uroot \
        -p"$(grep MYSQL_ROOT_PASSWORD .env | cut -d= -f2)" --silent 2>/dev/null; then
        # ping 通过后再确认能执行 SQL（初始化脚本可能还在运行）
        if docker-compose exec -T mysql mysql -uroot \
            -p"$(grep MYSQL_ROOT_PASSWORD .env | cut -d= -f2)" \
            -e "SELECT 1;" 2>/dev/null | grep -q 1; then
            echo " ✓"
            break
        fi
    fi
    echo -n "."
    sleep 3
done
REMOTE_INFRA
ok "MySQL + Redis 已就绪"

# ---------- 4. 服务器端：注册 / 重启 systemd 服务 ----------
step "4/4  服务器端：注册 systemd 服务并重启..."
ssh "${REMOTE_HOST}" bash <<REMOTE_SERVICE
set -e

# 写入 systemd 单元文件（幂等）
cat > /etc/systemd/system/${SERVICE_NAME}.service <<'UNIT'
[Unit]
Description=Freight Agent WeChat Bot
Documentation=https://github.com/your-org/freight-agent-wechat
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
User=root
WorkingDirectory=${REMOTE_DIR}
ExecStart=${REMOTE_DIR}/bin/${APP_NAME} --mode=pro
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
# 限制资源
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

chmod +x ${REMOTE_DIR}/bin/${APP_NAME}
systemctl daemon-reload
systemctl enable ${SERVICE_NAME} --quiet

# 优雅重启
if systemctl is-active --quiet ${SERVICE_NAME}; then
    systemctl restart ${SERVICE_NAME}
    echo "  服务已重启"
else
    systemctl start ${SERVICE_NAME}
    echo "  服务已启动"
fi

# 等待 2 秒确认启动状态
sleep 2
systemctl is-active --quiet ${SERVICE_NAME} && echo "  状态: running ✓" || {
    echo "  [错误] 服务未正常启动，查看日志："
    journalctl -u ${SERVICE_NAME} -n 20 --no-pager
    exit 1
}
REMOTE_SERVICE
ok "systemd 服务已注册并运行"

# ---------- 完成 ----------
echo ""
echo -e "${GREEN}══════════════════════════════════════════${NC}"
echo -e "${GREEN}  部署完成！${NC}"
echo -e "${GREEN}══════════════════════════════════════════${NC}"
echo ""
echo "  服务地址:   http://123.56.177.18:${PORT:-8080}/health"
echo ""
echo "  常用命令（在服务器执行）："
echo "  # 查看实时日志"
echo "  ssh ${REMOTE_HOST} 'journalctl -u ${SERVICE_NAME} -f'"
echo ""
echo "  # 查看服务状态"
echo "  ssh ${REMOTE_HOST} 'systemctl status ${SERVICE_NAME}'"
echo ""
echo "  # 手动重启"
echo "  ssh ${REMOTE_HOST} 'systemctl restart ${SERVICE_NAME}'"
echo ""
echo "  # 查看历史日志文件"
echo "  ssh ${REMOTE_HOST} 'tail -f ${REMOTE_DIR}/logs/app.log'"
