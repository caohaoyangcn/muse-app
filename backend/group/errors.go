package group

import "errors"

// Typed errors surfaced to the UI via UserMessage.
var (
	ErrEmptyName   = errors.New("group name is empty")
	ErrNotFound    = errors.New("group not found")
	ErrSelfParent  = errors.New("a group cannot be its own parent")
	ErrCycle       = errors.New("cannot set parent: would create a cycle")
)

// UserMessage maps group errors to the strings shown in the UI.
func UserMessage(err error) string {
	switch {
	case errors.Is(err, ErrEmptyName):
		return "分组名称不能为空"
	case errors.Is(err, ErrNotFound):
		return "找不到分组"
	case errors.Is(err, ErrSelfParent):
		return "分组不能设为自己或后代的父级"
	case errors.Is(err, ErrCycle):
		return "无法设置父级：会导致循环嵌套"
	default:
		return err.Error()
	}
}
