#!/bin/bash
# ============================================================
# freight-agent 服务管理脚本（在服务器上运行）
# 用法: ./service.sh [start|stop|restart|status|logs]
# ============================================================

SERVICE="freight-agent"

case "${1:-status}" in
  start)
    systemctl start "$SERVICE"
    echo "✓ 服务已启动"
    systemctl status "$SERVICE" --no-pager -l | head -20
    ;;
  stop)
    systemctl stop "$SERVICE"
    echo "✓ 服务已停止"
    ;;
  restart)
    systemctl restart "$SERVICE"
    echo "✓ 服务已重启"
    sleep 2
    systemctl status "$SERVICE" --no-pager -l | head -20
    ;;
  status)
    systemctl status "$SERVICE" --no-pager -l
    ;;
  logs)
    journalctl -u "$SERVICE" -f --no-pager
    ;;
  *)
    echo "用法: $0 [start|stop|restart|status|logs]"
    exit 1
    ;;
esac
