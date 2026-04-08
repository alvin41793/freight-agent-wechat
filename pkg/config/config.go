package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	LLM      LLMConfig      `mapstructure:"llm"`
	WeChat   WeChatConfig   `mapstructure:"wechat"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port         string `mapstructure:"port"`
	Mode         string `mapstructure:"mode"` // debug, release
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	Charset  string `mapstructure:"charset"`
}

// RedisConfig Redis配置
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// LLMConfig LLM配置
type LLMConfig struct {
	Provider       string  `mapstructure:"provider"` // openai, deepseek, qwen ...
	APIKey         string  `mapstructure:"api_key"`
	BaseURL        string  `mapstructure:"base_url"`
	Model          string  `mapstructure:"model"`
	Timeout        int     `mapstructure:"timeout"`
	MaxTokens      int     `mapstructure:"max_tokens"`
	Temperature    float64 `mapstructure:"temperature"`
	Thinking       bool    `mapstructure:"thinking"`        // 是否开启思考模式（支持 o-series / qwen3 / deepseek-reasoner 等）
	ThinkingBudget int     `mapstructure:"thinking_budget"` // 思考 token 上限（0 表示不限制）
}

// WeChatConfig 企业微信配置
type WeChatConfig struct {
	CorpID         string `mapstructure:"corp_id"`
	CorpSecret     string `mapstructure:"corp_secret"`
	AgentID        int    `mapstructure:"agent_id"`
	Token          string `mapstructure:"token"`
	EncodingAESKey string `mapstructure:"encoding_aes_key"`
	BotID          string `mapstructure:"bot_id"`
	BotSecret      string `mapstructure:"bot_secret"`
}

// DSN 生成数据库连接字符串
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.Charset)
}

// RedisAddr 生成Redis地址
func (r *RedisConfig) RedisAddr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

var globalConfig *Config

// LoadForMode 根据运行模式加载对应配置文件
// mode="debug" -> configs/config.debug.yaml
// mode="pro"   -> configs/config.pro.yaml
// 如果模式对应的文件不存在，回退到 configs/config.yaml
func LoadForMode(mode string) (*Config, error) {
	configFile := fmt.Sprintf("configs/config.%s.yaml", mode)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		log.Printf("[config] %s not found, falling back to configs/config.yaml", configFile)
		configFile = "configs/config.yaml"
	}
	return Load(configFile)
}

// Load 加载配置
func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	// 设置默认値
	setDefaults()

	// 读取环境变量（自动将 SERVER_PORT 映射到 server.port）
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// 显式绑定关键环境变量
	_ = viper.BindEnv("server.port", "SERVER_PORT")
	_ = viper.BindEnv("server.mode", "SERVER_MODE")
	_ = viper.BindEnv("llm.api_key", "LLM_API_KEY")
	_ = viper.BindEnv("llm.base_url", "LLM_BASE_URL")
	_ = viper.BindEnv("llm.model", "LLM_MODEL")
	_ = viper.BindEnv("redis.host", "REDIS_HOST")
	_ = viper.BindEnv("redis.port", "REDIS_PORT")
	_ = viper.BindEnv("redis.password", "REDIS_PASSWORD")
	_ = viper.BindEnv("database.host", "DB_HOST")
	_ = viper.BindEnv("database.port", "DB_PORT")
	_ = viper.BindEnv("database.user", "DB_USER")
	_ = viper.BindEnv("database.password", "DB_PASSWORD")
	_ = viper.BindEnv("database.dbname", "DB_NAME")
	_ = viper.BindEnv("wechat.corp_id", "WECHAT_CORP_ID")
	_ = viper.BindEnv("wechat.corp_secret", "WECHAT_CORP_SECRET")
	_ = viper.BindEnv("wechat.agent_id", "WECHAT_AGENT_ID")
	_ = viper.BindEnv("wechat.token", "WECHAT_TOKEN")
	_ = viper.BindEnv("wechat.encoding_aes_key", "WECHAT_ENCODING_AES_KEY")
	_ = viper.BindEnv("wechat.bot_id", "WECHAT_BOT_ID")
	_ = viper.BindEnv("wechat.bot_secret", "WECHAT_BOT_SECRET")

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: config file not found, using defaults and env vars: %v", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	globalConfig = &cfg
	return &cfg, nil
}

// Get 获取全局配置
func Get() *Config {
	if globalConfig == nil {
		panic("config not loaded")
	}
	return globalConfig
}

// setDefaults 设置默认值
func setDefaults() {
	viper.SetDefault("server.port", "8080")
	viper.SetDefault("server.mode", "release")
	viper.SetDefault("server.read_timeout", 30)
	viper.SetDefault("server.write_timeout", 30)

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 3306)
	viper.SetDefault("database.charset", "utf8mb4")

	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("llm.provider", "deepseek")
	viper.SetDefault("llm.model", "deepseek-chat")
	viper.SetDefault("llm.timeout", 60)
	viper.SetDefault("llm.max_tokens", 4000)
	viper.SetDefault("llm.temperature", 0.3)
}
