FROM node:22-slim

# Install build dependencies (required by openclaw plugin native modules)
# Use USTC mirror for faster downloads in China
RUN rm -f /etc/apt/sources.list.d/debian.sources && \
    echo "deb http://mirrors.ustc.edu.cn/debian bookworm main\ndeb http://mirrors.ustc.edu.cn/debian bookworm-updates main\ndeb http://mirrors.ustc.edu.cn/debian-security bookworm-security main" > /etc/apt/sources.list && \
    apt-get update && apt-get install -y python3 make g++ --no-install-recommends && rm -rf /var/lib/apt/lists/*

# Install openclaw globally
RUN npm install -g openclaw@latest

# Create directory structure
RUN mkdir -p /root/.openclaw/workspace \
    /root/.openclaw/workspace-freight-bot \
    /root/.openclaw/agents/freight-bot/agent \
    /root/.openclaw/extensions

# Step 1: 用最简 config 安装插件（避免 wecom 频道未注册导致的 config 校验失败）
RUN echo '{"agents":{"defaults":{"workspace":"/root/.openclaw/workspace"},"list":[{"id":"main"}]},"plugins":{"allow":[],"load":{"paths":[]},"entries":{},"installs":{}}}' \
    > /root/.openclaw/openclaw.json

RUN openclaw plugins install @wecom/wecom-openclaw-plugin 2>&1; exit 0
RUN openclaw plugins install @larksuite/openclaw-lark 2>&1; exit 0

# Step 2: 覆盖为完整 config（插件已安装，wecom 频道可以识别）
COPY config/openclaw.json /root/.openclaw/openclaw.json

# Copy freight-bot agent config
COPY config/auth-profiles.json /root/.openclaw/agents/freight-bot/agent/auth-profiles.json
COPY config/models.json /root/.openclaw/agents/freight-bot/agent/models.json

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 18789

ENTRYPOINT ["/entrypoint.sh"]
