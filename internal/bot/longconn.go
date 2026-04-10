package bot

import (
	"context"
	"log"
	"time"

	wecomaibot "github.com/Area1m/wecom-aibot-go"
)

// LongConnConfig 长连接配置
type LongConnConfig struct {
	BotID     string
	BotSecret string
}

// MessageHandler 消息处理接口，由 handler.FreightAgentHandler 实现
// 返回空字符串表示不回复
type MessageHandler interface {
	Handle(ctx context.Context, userID, chatID, text string) string
}

// StartLongConnection 启动企业微信智能机器人长连接
// 使用官方 Go SDK（github.com/Area1m/wecom-aibot-go）
// WebSocket 地址: wss://openws.work.weixin.qq.com
// SDK 内置指数退避重连（默认最多 10 次），传入 MaxReconnectAttempts: -1 则无限重连
func StartLongConnection(cfg LongConnConfig, handler MessageHandler) {
	if cfg.BotID == "" || cfg.BotSecret == "" {
		log.Fatal("[longconn] bot_id 或 bot_secret 未配置，请检查 configs/config.{mode}.yaml")
	}

	client := wecomaibot.NewClient(wecomaibot.Config{
		BotID:                cfg.BotID,
		Secret:               cfg.BotSecret,
		MaxReconnectAttempts: -1, // 无限重连
		HeartbeatIntervalMS:  30000,
	})

	// 连接生命周期日志
	client.OnConnected(func(ctx context.Context) {
		log.Println("[longconn] WebSocket connected")
	})
	client.OnAuthenticated(func(ctx context.Context) {
		log.Println("[longconn] authenticated, waiting for messages...")
	})
	client.OnDisconnected(func(ctx context.Context, reason string) {
		log.Printf("[longconn] disconnected: %s", reason)
	})
	client.OnReconnecting(func(ctx context.Context, attempt int) {
		log.Printf("[longconn] reconnecting (attempt %d)...", attempt)
	})
	client.OnError(func(ctx context.Context, err error) {
		log.Printf("[longconn] error: %v", err)
	})

	// 处理文本消息
	client.OnText(func(ctx context.Context, msg wecomaibot.TextMessage) {
		userID := msg.From.UserID
		chatID := msg.ChatID
		text := msg.Text.Content

		log.Printf("[longconn] msg from user=%s chat=%s text=%q", userID, chatID, text)

		// 生成本次回复的流式 ID，整个过程共用同一个 ID
		streamID := wecomaibot.GenerateReqID("stream")

		// 立即发送静态占位符（finish:false），使用多行占位减少长度差异，降低重排抖动
		placeholder := "AI正在解析中..."
		if _, err := client.ReplyStreamByID(msg, streamID, placeholder, false, nil, nil); err != nil {
			log.Printf("[longconn] stream start error: %v", err)
		}

		// 异步处理 LLM，完成后一次性返回最终表格
		go func() {
			start := time.Now()
			reply := handler.Handle(ctx, userID, chatID, text)
			log.Printf("[longconn] handle done user=%s elapsed=%s", userID, time.Since(start).Round(time.Millisecond))

			if reply == "" {
				reply = "处理完成。"
			}
			// finish:true 结束流，显示最终回复
			if _, err := client.ReplyStreamByID(msg, streamID, reply, true, nil, nil); err != nil {
				log.Printf("[longconn] reply error: %v", err)
			}
		}()
	})

	// 启动长连接（内部自动重连）
	if err := client.Connect(); err != nil {
		log.Fatalf("[longconn] connect failed: %v", err)
	}

	// 阻塞保持运行
	select {}
}
