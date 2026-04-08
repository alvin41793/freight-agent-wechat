# 货运报价单 AI 系统

企业微信群机器人 + AI Agent，支持根据用户文字输入自动生成格式精美、可直接对外发送的运价单。

支持三种输出格式：
- 图片（推荐，企业微信体验最佳）
- Excel（正式场景）
- 文本（备用）

## 核心定位

**这不是 Agent 系统，而是：**
> 一个带状态的报价单编辑器 + LLM 作为自然语言接口

## 系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                      企业微信客户端                           │
└───────────────────────┬─────────────────────────────────────┘
                        │ Webhook/回调
┌───────────────────────▼─────────────────────────────────────┐
│                    会话接入层 (Bot)                           │
│  - 消息接收/发送                                              │
│  - 用户身份识别                                               │
│  - 消息类型处理（文本/图片/文件）                              │
└───────────────────────┬─────────────────────────────────────┘
                        │
┌───────────────────────▼─────────────────────────────────────┐
│                   会话状态管理器 (核心)                        │
│  - Session 生命周期管理                                       │
│  - 当前报价单状态 (CurrentQuote)                              │
│  - 历史版本链 (History)                                       │
│  - 意图路由 (Intent Router)                                   │
└───────────┬───────────────────────────┬─────────────────────┘
            │                           │
┌───────────▼───────────┐   ┌───────────▼───────────┐
│     LLM 解析服务       │   │     渲染引擎服务       │
│  - 意图识别            │   │  - HTML 模板渲染       │
│  - 结构提取            │   │  - 图片生成 (chromedp) │
│  - 修改指令生成        │   │  - Excel 导出          │
│  - Schema 约束         │   │  - PDF 生成            │
└───────────┬───────────┘   └───────────┬───────────┘
            │                           │
└───────────▼───────────────────────────▼─────────────────────┐
│                    数据存储层                                │
│  - MySQL: 报价单持久化存储                                   │
│  - Redis: 会话缓存 (TTL: 30min)                              │
└─────────────────────────────────────────────────────────────┘
```

## 功能特性

### 核心功能
- 文本输入解析：自然语言转结构化报价数据
- 多轮对话修改：支持增量修改报价单
- 历史版本管理：可查看和回滚历史版本
- 多格式导出：图片/Excel/文本

### 支持的意图
- `create_quote`: 创建新报价单
- `update_quote`: 修改现有报价单
- `explain`: 解释当前报价
- `export`: 导出报价单
- `rollback`: 回滚版本
- `clear`: 清空会话

## 快速开始

### 前置条件
- Go 1.24+
- Docker & Docker Compose
- 企业微信机器人配置
- LLM API Key（DeepSeek/OpenAI）

### 本地开发

```bash
# 克隆项目
git clone <repository-url>
cd freight-agent-wechat

# 安装依赖
go mod download

# 复制配置文件
cp configs/config.yaml configs/config.local.yaml
cp .env.example .env

# 编辑配置文件，填入实际值
vim configs/config.yaml

# 启动依赖服务
docker-compose up -d redis mysql

# 运行服务
make run
# 或

go run ./cmd/server
```

### Docker 部署

```bash
# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f app
```

### 测试接口

```bash
# 健康检查
curl http://localhost:8080/health

# 测试消息处理（debug 模式）
curl -X POST http://localhost:8080/test \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "test_user",
    "group_id": "test_group",
    "message": "上海到洛杉矶，20GP 1000，40HQ 1800"
  }'
```

## 使用示例

### 创建报价单
```
用户: 上海到洛杉矶，20GP 1000，40HQ 1800，附加费200

系统: 当前报价单摘要：
━━━━━━━━━━━━━━━━
报价单号：Q20260320123456ABCD
航线：上海 → 洛杉矶
━━━━━━━━━━━━━━━━
20GP: USD 1000
40HQ: USD 1800
━━━━━━━━━━━━━━━━
总计：USD 3000.00
━━━━━━━━━━━━━━━━

如需修改，请直接输入指令。
如需导出，请说 '导出图片' 或 '导出Excel'。
```

### 修改报价单
```
用户: 40HQ 改成 2000

系统: 已更新 40HQ 价格。
当前报价单摘要：
...
```

### 导出报价单
```
用户: 导出图片

系统: [发送报价单图片]
```

## 项目结构

```
freight-agent-wechat/
├── cmd/
│   └── server/              # 主入口
│       └── main.go
├── internal/
│   ├── bot/                 # 企业微信机器人
│   │   ├── webhook.go       # 消息接收
│   │   └── message.go       # 消息格式化
│   ├── session/             # 会话状态管理
│   │   ├── manager.go       # Session 管理器
│   │   ├── store.go         # 存储接口
│   │   └── model.go         # 会话模型
│   ├── llm/                 # LLM 服务
│   │   ├── client.go        # 通用客户端
│   │   ├── parser.go        # 报价解析
│   │   └── intent.go        # 意图识别
│   ├── quote/               # 报价单业务
│   │   └── model.go         # 数据模型
│   ├── renderer/            # 渲染引擎
│   │   ├── html.go          # HTML 模板渲染
│   │   ├── image.go         # 图片生成
│   │   └── excel.go         # Excel 导出
│   └── handler/             # HTTP 处理器
│       └── webhook.go
├── pkg/
│   └── config/              # 配置管理
├── configs/
│   └── config.yaml          # 配置文件
├── migrations/              # 数据库迁移
├── docker-compose.yml
├── Dockerfile
├── go.mod
└── README.md
```

## 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 后端框架 | Go + Gin | 轻量高效 |
| ORM | GORM | MySQL 操作 |
| 配置 | Viper | 环境变量 + 配置文件 |
| LLM | DeepSeek / OpenAI | JSON Schema + Function Calling |
| 模板引擎 | Go html/template | 原生支持 |
| CSS | Tailwind CDN | 快速样式 |
| 图片生成 | chromedp | Headless Chrome 截图 |
| Excel | excelize | Go 原生 Excel 操作 |
| 缓存 | Redis | 会话状态（TTL 30min） |
| 数据库 | MySQL | 报价单持久化 |
| 部署 | Docker + Docker Compose | 一键启动 |

## 配置说明

### 环境变量

| 变量名 | 说明 | 必填 |
|--------|------|------|
| LLM_API_KEY | LLM API 密钥 | 是 |
| LLM_BASE_URL | LLM API 地址 | 否 |
| WECHAT_CORP_ID | 企业微信 CorpID | 是 |
| WECHAT_CORP_SECRET | 企业微信 Secret | 是 |
| WECHAT_AGENT_ID | 应用 AgentID | 是 |
| DB_PASSWORD | 数据库密码 | 是 |
| REDIS_PASSWORD | Redis 密码 | 否 |

## 开发路线

### Phase 1: 核心闭环 (已完成)
- [x] 企业微信 Webhook 接入
- [x] 基础会话管理
- [x] LLM 解析（创建 + 修改）
- [x] HTML 模板渲染
- [x] 图片生成与发送

### Phase 2: 增强体验
- [ ] Excel 导出优化
- [ ] 历史版本查看 UI
- [ ] 版本回滚优化
- [ ] 常用模板（"和上次一样"）

### Phase 3: 智能功能
- [ ] 价格异常检测
- [ ] 附加费智能建议
- [ ] 多模板风格切换

## License

MIT

# 正确用法
./scripts/build.sh              # 编译 linux/amd64（服务器）
./scripts/build.sh darwin arm64 # 编译本地 macOS Apple Silicon

# 传错了立即报错
./scripts/build.sh debug
# [ERROR] 不支持的 OS: debug
#         合法值: linux | darwin | windows
#         注意: 运行模式（debug/pro）在启动时指定，不在编译时指定
#         启动: ./scripts/start.sh [debug|pro]