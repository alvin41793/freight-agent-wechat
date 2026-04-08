package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// contextKey 未导出类型，防止与其他包的 key 冲突
type contextKey string

// Context key 常量，用于在 context.Context 中传递用户信息
const (
	ContextKeyUserID contextKey = "llm_user_id"
	ContextKeyChatID contextKey = "llm_chat_id"
)

// LLMEntry 记录单次 LLM 调用的输入/输出，用于 debug 模式持久化
type LLMEntry struct {
	Time             time.Time `json:"time"`
	Function         string    `json:"function"`                    // ParseQuoteInput / SupplementRates
	UserID           string    `json:"user_id,omitempty"`           // 发送者
	ChatID           string    `json:"chat_id,omitempty"`           // 来源群/会话
	UserInput        string    `json:"user_input"`                  // 用户原始输入
	Context          string    `json:"context,omitempty"`           // 附加上下文（如待补充运价 JSON）
	ReasoningContent string    `json:"reasoning_content,omitempty"` // 模型思考过程（思考模式开启时）
	Output           string    `json:"output,omitempty"`            // 模型输出（JSON 字符串）
	DurationMS       int64     `json:"duration_ms"`                 // 调用耗时（毫秒）
	Error            string    `json:"error,omitempty"`             // 错误信息
}

// Logger 日志模块：debug 模式下将 LLM 交互记录写入文件
type Logger struct {
	mode   string              // "debug" 或 "pro"
	logDir string              // 日志根目录，如 "logs"
	mu     sync.Mutex          // 保护 files map
	files  map[string]*os.File // date(2006-01-02) -> 日志文件句柄
}

// New 创建 Logger
// mode:   "debug" 或 "pro"
// logDir: 日志根目录（如 "logs"）
func New(mode, logDir string) *Logger {
	return &Logger{
		mode:   mode,
		logDir: logDir,
		files:  make(map[string]*os.File),
	}
}

// IsDebug 是否为 debug 模式
func (l *Logger) IsDebug() bool {
	return l.mode == "debug"
}

// LogLLM 将 LLM 交互记录写入当天日志文件（debug 模式有效，pro 模式直接返回）
// 日志路径：{logDir}/{YYYY-MM-DD}/llm_{YYYY-MM-DD}.jsonl
// 每条记录为一行 JSON（JSONL 格式）
func (l *Logger) LogLLM(entry *LLMEntry) {
	if !l.IsDebug() {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	f, err := l.getOrCreateFile(today)
	if err != nil {
		fmt.Printf("[logger] cannot open log file: %v\n", err)
		return
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = f.WriteString(string(data) + "\n")
}

// getOrCreateFile 获取或创建指定日期的日志文件（调用方需持有锁）
func (l *Logger) getOrCreateFile(date string) (*os.File, error) {
	if f, ok := l.files[date]; ok {
		return f, nil
	}

	dir := filepath.Join(l.logDir, date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir %s: %w", dir, err)
	}

	filename := filepath.Join(dir, fmt.Sprintf("llm_%s.jsonl", date))
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", filename, err)
	}

	l.files[date] = f
	return f, nil
}

// Close 关闭所有打开的日志文件
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, f := range l.files {
		_ = f.Close()
	}
	l.files = make(map[string]*os.File)
}
