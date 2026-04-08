#!/bin/bash
set -e

MINIMAL_CONFIG='{"agents":{"defaults":{"workspace":"/root/.openclaw/workspace"},"list":[{"id":"main"}]},"plugins":{"allow":[],"load":{"paths":[]},"entries":{},"installs":{}}}'

install_plugin() {
    local plugin=$1
    echo "安装插件: $plugin"
    for i in $(seq 1 5); do
        if openclaw plugins install "$plugin" 2>&1; then
            echo "$plugin 安装成功"
            return 0
        fi
        echo "$plugin 安装失败 (第 $i/5 次)，等待 20 秒重试..."
        sleep 20
    done
    echo "警告: $plugin 安装失败，将继续启动"
    return 0
}

# 首次启动时安装插件（使用最小 config 避免 wecom 频道校验失败）
if [ ! -d "/root/.openclaw/extensions/wecom-openclaw-plugin" ]; then
    echo "=== 首次启动，安装 wecom-openclaw-plugin ==="
    cp /root/.openclaw/openclaw.json /root/.openclaw/openclaw.json.bak
    echo "$MINIMAL_CONFIG" > /root/.openclaw/openclaw.json
    install_plugin "@wecom/wecom-openclaw-plugin"
    cp /root/.openclaw/openclaw.json.bak /root/.openclaw/openclaw.json
fi

echo "=== 启动 OpenClaw ==="
exec openclaw gateway run
