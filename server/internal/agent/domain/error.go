// Package domain 定义 Agent 对外稳定错误。
package domain

// Error 是可安全返回给客户端的 Agent 错误。
type Error struct {
	Code      string `json:"code"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Error 实现 error 接口。
func (e Error) Error() string {
	return e.Message
}

// NewError 创建稳定的 Agent 错误值。
func NewError(code, kind, message string, retryable bool) Error {
	return Error{
		Code:      code,
		Kind:      kind,
		Message:   message,
		Retryable: retryable,
	}
}
