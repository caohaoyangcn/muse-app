package watch

import "errors"

// Typed validation errors, mirrored by UserMessage for the UI.
var (
	ErrEmptyPath      = errors.New("watch directory path is empty")
	ErrNotExist       = errors.New("directory does not exist")
	ErrNotDir         = errors.New("path is not a directory")
	ErrAlreadyWatched = errors.New("directory is already being watched")
	ErrNotFound       = errors.New("watched directory not found")
)

// UserMessage maps errors to the strings shown in the UI.
func UserMessage(err error) string {
	switch {
	case errors.Is(err, ErrEmptyPath):
		return "目录路径为空"
	case errors.Is(err, ErrNotExist):
		return "目录不存在"
	case errors.Is(err, ErrNotDir):
		return "该路径不是文件夹"
	case errors.Is(err, ErrAlreadyWatched):
		return "该目录已在监控清单中"
	case errors.Is(err, ErrNotFound):
		return "找不到该监控目录"
	default:
		return err.Error()
	}
}
