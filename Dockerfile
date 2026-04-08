# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 安装依赖
RUN apk add --no-cache git ca-certificates tzdata

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /server ./cmd/server

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata chromium

# 设置时区
ENV TZ=Asia/Shanghai

# 从构建阶段复制二进制文件
COPY --from=builder /server .
COPY --from=builder /app/configs ./configs

# 创建非 root 用户
RUN adduser -D -g '' appuser
USER appuser

# 暴露端口
EXPOSE 8080

# 启动服务
CMD ["./server"]
