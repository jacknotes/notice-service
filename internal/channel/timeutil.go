package channel

import "time"

func nowUnix() int64 { return time.Now().UnixMilli() }
