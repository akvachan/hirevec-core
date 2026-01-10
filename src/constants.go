// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

import "time"

const (
	Bit      int64 = 1
	Kilobyte int64 = Bit * 1024
	Megabyte int64 = Kilobyte * 1024
	Gigabyte int64 = Megabyte * 1024
	Terabyte int64 = Gigabyte * 1024
)

const (
	PageSizeDefaultLimit = 50
	PageSizeMaxLimit     = 100
)

const (
	MaxBytesHandler = 1 * Megabyte
	ReadTimeout     = 2 * time.Second
	WriteTimout     = 2 * time.Second
)

const (
	Addr = "localhost:8888"
)
