package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config 平台服务端配置，全部通过 .env / 环境变量注入
type Config struct {
	Port                  int    // HTTP 监听端口
	AdminPassword         string // 管理密码（必填）
	JWTSecret             string // JWT 签名密钥，缺省时由管理密码派生
	DBPath                string // SQLite 数据库文件路径
	SharedFilesRoot       string // 共享文件根目录，每个项目一个子目录
	PlatformPublicURL     string // 平台对外可达地址，写入下发给 Hermes 的指令上下文
	RecentMessagesLimit   int    // 指令上下文携带的最近消息条数
	DispatchTimeoutMinute int    // 派发任务超时（分钟）
}

// Load 读取 .env 文件（若存在）并加载配置
func Load(envFile string) (*Config, error) {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			return nil, fmt.Errorf("加载 env 文件 %s 失败: %w", envFile, err)
		}
	} else {
		_ = godotenv.Load() // 默认尝试当前目录 .env，不存在则忽略
	}

	cfg := &Config{
		Port:                  getInt("PORT", 8080),
		AdminPassword:         os.Getenv("ADMIN_PASSWORD"),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		DBPath:                getStr("DB_PATH", "./data/hermeshub.db"),
		SharedFilesRoot:       getStr("SHARED_FILES_ROOT", "./data/shared"),
		PlatformPublicURL:     getStr("PLATFORM_PUBLIC_URL", "http://localhost:8080"),
		RecentMessagesLimit:   getInt("RECENT_MESSAGES_LIMIT", 20),
		DispatchTimeoutMinute: getInt("DISPATCH_TIMEOUT_MINUTES", 30),
	}

	if cfg.AdminPassword == "" {
		return nil, fmt.Errorf("必须在 .env 或环境变量中设置 ADMIN_PASSWORD")
	}
	if cfg.JWTSecret == "" {
		// 由管理密码派生，保证重启后 token 仍有效
		sum := sha256.Sum256([]byte("hermeshub-jwt:" + cfg.AdminPassword))
		cfg.JWTSecret = hex.EncodeToString(sum[:])
	}
	return cfg, nil
}

func getStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
