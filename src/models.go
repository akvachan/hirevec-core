// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

type Position struct {
	PositionID  int    `json:"position_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Company     string `json:"company"`
}
