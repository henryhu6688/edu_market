package config

import (
	"fmt"
	"log"

	"github.com/spf13/viper"
)

// Config 全局配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	AI       AIConfig       `mapstructure:"ai"`
	Upload   UploadConfig   `mapstructure:"upload"`
	Captcha  CaptchaConfig  `mapstructure:"captcha"`
}

// ServerConfig 服务配置
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	Charset      string `mapstructure:"charset"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
}

// DSN 返回 MySQL 连接字符串
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.Charset)
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"`
}

// AIConfig AI 配置
type AIConfig struct {
	Provider string `mapstructure:"provider"`
	APIKey   string `mapstructure:"api_key"`
	APIURL   string `mapstructure:"api_url"`
	Model    string `mapstructure:"model"`
}

// CaptchaConfig 验证码配置
type CaptchaConfig struct {
	Length         int `mapstructure:"length"`
	ExpireSeconds  int `mapstructure:"expire_seconds"`
	ResendSeconds  int `mapstructure:"resend_seconds"`
}

// UploadConfig 上传配置
type UploadConfig struct {
	MaxSize     int64    `mapstructure:"max_size"`
	AllowedExts []string `mapstructure:"allowed_exts"`
}

// App 全局配置实例
var App *Config

// Load 加载配置文件
func Load() {
	viper.SetConfigName("app")
	viper.SetConfigType("yml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	App = &Config{}
	if err := viper.Unmarshal(App); err != nil {
		log.Fatalf("解析配置文件失败: %v", err)
	}

	log.Printf("配置加载成功，运行模式: %s, 端口: %d", App.Server.Mode, App.Server.Port)
}
