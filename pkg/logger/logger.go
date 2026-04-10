package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// LLMEntry 记录单次 LLM 调用的输入/输出
type LLMEntry struct {
	Time             time.Time `json:"time"`
	Function         string    `json:"function"`                    // ParseQuoteInput / SupplementRates
	UserID           string    `json:"user_id,omitempty"`           // 发送者
	ChatID           string    `json:"chat_id,omitempty"`           // 来源群/会话
	SystemPrompt     string    `json:"system_prompt"`               // 完整系统提示词
	UserInput        string    `json:"user_input"`                  // 用户原始输入
	Context          string    `json:"context,omitempty"`           // 附加上下文（如待补充运价 JSON）
	ReasoningContent string    `json:"reasoning_content,omitempty"` // 模型思考过程（思考模式开启时）
	Output           string    `json:"output,omitempty"`            // 模型输出（JSON 字符串）
	DurationMS       int64     `json:"duration_ms"`                 // 调用耗时（毫秒）
	Error            string    `json:"error,omitempty"`             // 错误信息
}

// Logger 日志模块
type Logger struct {
	mode   string // "debug" 或 "pro"
	logDir string // 日志根目录

	// Debug 模式: LLM 完整调用日志
	muDebug    sync.Mutex
	debugFiles map[string]*os.File // filename -> 文件句柄

	// Production 模式: 分级日志
	muProd   sync.Mutex
	infoFile *os.File // app.log
	errFile  *os.File // error.log
}

// New 创建 Logger
// mode:   "debug" 或 "pro"
// logDir: 日志根目录（如 "logs"）
func New(mode, logDir string) *Logger {
	return &Logger{
		mode:       mode,
		logDir:     logDir,
		debugFiles: make(map[string]*os.File),
	}
}

// IsDebug 是否为 debug 模式
func (l *Logger) IsDebug() bool {
	return l.mode == "debug"
}

// ========================================
// Debug 模式: 完整 LLM 调用日志 (TXT 格式)
// ========================================

// LogLLMDebug 记录完整 LLM 调用日志(TXT 格式,仅 debug 模式)
func (l *Logger) LogLLMDebug(entry *LLMEntry) {
	if !l.IsDebug() {
		return
	}

	l.muDebug.Lock()
	defer l.muDebug.Unlock()

	today := time.Now().Format("2006-01-02")
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("llm_%s_%s.txt", today, timestamp)

	f, err := l.getOrCreateDebugFile(today, filename)
	if err != nil {
		fmt.Printf("[logger] cannot open debug log file: %v\n", err)
		return
	}

	content := l.formatLLMEntry(entry)
	_, _ = f.WriteString(content)
}

// formatLLMEntry 格式化为可读 TXT
func (l *Logger) formatLLMEntry(entry *LLMEntry) string {
	var sb strings.Builder
	sb.WriteString("========================================\n")
	sb.WriteString("LLM Call Log\n")
	sb.WriteString("========================================\n")
	sb.WriteString(fmt.Sprintf("Time: %s\n", entry.Time.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Function: %s\n", entry.Function))
	if entry.UserID != "" {
		sb.WriteString(fmt.Sprintf("User ID: %s\n", entry.UserID))
	}
	if entry.ChatID != "" {
		sb.WriteString(fmt.Sprintf("Chat ID: %s\n", entry.ChatID))
	}
	sb.WriteString(fmt.Sprintf("Duration: %dms\n", entry.DurationMS))
	sb.WriteString("\n")

	if entry.SystemPrompt != "" {
		sb.WriteString("----------------------------------------\n")
		sb.WriteString("System Prompt:\n")
		sb.WriteString("----------------------------------------\n")
		sb.WriteString(entry.SystemPrompt + "\n\n")
	}

	sb.WriteString("----------------------------------------\n")
	sb.WriteString("User Input:\n")
	sb.WriteString("----------------------------------------\n")
	sb.WriteString(entry.UserInput + "\n\n")

	if entry.Context != "" {
		sb.WriteString("----------------------------------------\n")
		sb.WriteString("Context:\n")
		sb.WriteString("----------------------------------------\n")
		sb.WriteString(entry.Context + "\n\n")
	}

	if entry.ReasoningContent != "" {
		sb.WriteString("----------------------------------------\n")
		sb.WriteString("Reasoning Content:\n")
		sb.WriteString("----------------------------------------\n")
		sb.WriteString(entry.ReasoningContent + "\n\n")
	}

	sb.WriteString("----------------------------------------\n")
	sb.WriteString("Model Output (JSON):\n")
	sb.WriteString("----------------------------------------\n")
	if entry.Output != "" {
		// 尝试格式化 JSON
		var jsonObj interface{}
		if err := json.Unmarshal([]byte(entry.Output), &jsonObj); err == nil {
			if formatted, err := json.MarshalIndent(jsonObj, "", "  "); err == nil {
				sb.WriteString(string(formatted) + "\n\n")
			} else {
				sb.WriteString(entry.Output + "\n\n")
			}
		} else {
			sb.WriteString(entry.Output + "\n\n")
		}
	} else {
		sb.WriteString("(empty)\n\n")
	}

	if entry.Error != "" {
		sb.WriteString("----------------------------------------\n")
		sb.WriteString("Status: ERROR\n")
		sb.WriteString(fmt.Sprintf("Error: %s\n", entry.Error))
	} else {
		sb.WriteString("----------------------------------------\n")
		sb.WriteString("Status: SUCCESS\n")
	}
	sb.WriteString("========================================\n\n")

	return sb.String()
}

// getOrCreateDebugFile 获取或创建 debug 日志文件（调用方需持有锁）
func (l *Logger) getOrCreateDebugFile(date, filename string) (*os.File, error) {
	if f, ok := l.debugFiles[filename]; ok {
		return f, nil
	}

	dir := filepath.Join(l.logDir, date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir %s: %w", dir, err)
	}

	filepath := filepath.Join(dir, filename)
	f, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", filepath, err)
	}

	l.debugFiles[filename] = f
	return f, nil
}

// ========================================
// Production 模式: 分级日志
// ========================================

// Info 记录 INFO 级别日志(debug 和 pro 模式都有效)
func (l *Logger) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf("[INFO] %s %s",
		time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf(format, args...))

	l.muProd.Lock()
	defer l.muProd.Unlock()

	if l.infoFile == nil {
		if err := l.initProdLogs(); err != nil {
			fmt.Printf("[logger] init prod logs error: %v\n", err)
			return
		}
	}

	_, _ = l.infoFile.WriteString(msg + "\n")
}

// Warn 记录 WARN 级别日志(debug 和 pro 模式都有效)
func (l *Logger) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf("[WARN] %s %s",
		time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf(format, args...))

	l.muProd.Lock()
	defer l.muProd.Unlock()

	if l.infoFile == nil {
		if err := l.initProdLogs(); err != nil {
			fmt.Printf("[logger] init prod logs error: %v\n", err)
			return
		}
	}

	_, _ = l.infoFile.WriteString(msg + "\n")
}

// Error 记录 ERROR 级别日志(debug 和 pro 模式都有效,同时写入 app.log 和 error.log)
func (l *Logger) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf("[ERROR] %s %s",
		time.Now().Format("2006-01-02 15:04:05"),
		fmt.Sprintf(format, args...))

	l.muProd.Lock()
	defer l.muProd.Unlock()

	if l.infoFile == nil {
		if err := l.initProdLogs(); err != nil {
			fmt.Printf("[logger] init prod logs error: %v\n", err)
			return
		}
	}

	// ERROR 同时写入 app.log 和 error.log
	_, _ = l.infoFile.WriteString(msg + "\n")
	_, _ = l.errFile.WriteString(msg + "\n")
}

// initProdLogs 初始化生产日志文件
func (l *Logger) initProdLogs() error {
	if err := os.MkdirAll(l.logDir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	infoPath := filepath.Join(l.logDir, "app.log")
	errPath := filepath.Join(l.logDir, "error.log")

	var err error
	l.infoFile, err = os.OpenFile(infoPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open info log: %w", err)
	}

	l.errFile, err = os.OpenFile(errPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open error log: %w", err)
	}

	return nil
}

// ========================================
// 通用方法
// ========================================

// Close 关闭所有打开的日志文件
func (l *Logger) Close() {
	// 关闭 debug 文件
	l.muDebug.Lock()
	for _, f := range l.debugFiles {
		_ = f.Close()
	}
	l.debugFiles = make(map[string]*os.File)
	l.muDebug.Unlock()

	// 关闭生产日志文件
	l.muProd.Lock()
	if l.infoFile != nil {
		_ = l.infoFile.Close()
		l.infoFile = nil
	}
	if l.errFile != nil {
		_ = l.errFile.Close()
		l.errFile = nil
	}
	l.muProd.Unlock()
}
