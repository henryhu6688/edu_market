package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// MessageRole 定义
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall JSON 结构（存 messages.tool_calls 字段）
type ToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Result    string `json:"result,omitempty"`
}

// ToolCalls 实现 sql.Scanner / driver.Valuer
type ToolCalls []ToolCall

func (tc *ToolCalls) Scan(value interface{}) error {
	if value == nil {
		*tc = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, tc)
}

func (tc ToolCalls) Value() (driver.Value, error) {
	if tc == nil {
		return nil, nil
	}
	return json.Marshal(tc)
}

// Message AI 对话消息模型
type Message struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID  uint      `gorm:"not null;index" json:"session_id"`
	Role       string    `gorm:"type:varchar(20);not null" json:"role"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	ToolCalls  ToolCalls `gorm:"type:json;default:null" json:"tool_calls,omitempty"`
	TokensUsed int       `gorm:"default:0" json:"tokens_used"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	Session Session `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
}

// TableName 指定表名
func (Message) TableName() string {
	return "messages"
}
