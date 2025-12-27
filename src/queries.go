// Copyright (c) 2025 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

var GetPositionByIDQuery string = `
	SELECT COALESCE(json_agg(t), '[]'::json)
	FROM hirevec.general.positions t
	WHERE t.position_id = $1
`

var GetPositionsQuery string = `
	SELECT COALESCE(json_agg(t), '[]'::json)
	FROM (
		SELECT *
		FROM hirevec.general.positions
		ORDER BY position_id
		LIMIT $1 OFFSET $2
	) t
`
