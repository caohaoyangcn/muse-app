package video

import "time"

// nowMillis is the wall-clock timestamp used for all INTEGER time columns.
func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// nowNanos is the nanosecond timestamp used to name generated files. It only has
// to make two names in one library distinct, not to be a clock anyone reads.
func nowNanos() int64 {
	return time.Now().UnixNano()
}
