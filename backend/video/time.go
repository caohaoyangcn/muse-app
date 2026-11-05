package video

import "time"

// nowMillis is the wall-clock timestamp used for all INTEGER time columns.
func nowMillis() int64 {
	return time.Now().UnixMilli()
}
