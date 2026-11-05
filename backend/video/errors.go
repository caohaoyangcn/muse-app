package video

import (
	"errors"
)

// Typed validation errors (spec-v1.md §29 "User Error" category).
var (
	ErrEmptyPath          = errors.New("file path is empty")
	ErrNotExist           = errors.New("file does not exist")
	ErrNotRegularFile     = errors.New("not a regular file")
	ErrSymlink            = errors.New("symbolic links are not supported")
	ErrFolderNotSupported = errors.New("folders are not supported yet; please drop video files instead")
	ErrUnsupportedType    = errors.New("unsupported file type")
	ErrAlreadyAdded       = errors.New("this video is already in favorites")
	ErrNotFound           = errors.New("video not found")
	// ErrFingerprinterUnavailable is returned when content dedup is asked for
	// before the fingerprint worker has been wired up.
	ErrFingerprinterUnavailable = errors.New("content fingerprinting is not available")
)

// UserMessage maps errors to the strings shown in the UI.
func UserMessage(err error) string {
	switch {
	case errors.Is(err, ErrEmptyPath):
		return "文件路径为空"
	case errors.Is(err, ErrNotExist):
		return "文件不存在"
	case errors.Is(err, ErrNotRegularFile):
		return "不是常规文件"
	case errors.Is(err, ErrSymlink):
		return "不支持符号链接"
	case errors.Is(err, ErrFolderNotSupported):
		return "暂不支持文件夹，请拖入视频文件"
	case errors.Is(err, ErrUnsupportedType):
		return "不支持的文件类型"
	case errors.Is(err, ErrAlreadyAdded):
		return "已在收藏中"
	case errors.Is(err, ErrNotFound):
		return "找不到视频"
	default:
		return err.Error()
	}
}
