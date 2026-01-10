// Copyright (c) 2026 Arsenii Kvachan. All Rights Reserved. MIT License.

package hirevec

var GetPositionByIDQuery string = `
	SELECT COALESCE(json_agg(t), '[]'::json)
	FROM hirevec.general.positions t
	WHERE t.id = $1
`

var GetPositionsQuery string = `
	SELECT COALESCE(json_agg(t), '[]'::json)
	FROM (
		SELECT *
		FROM hirevec.general.positions
		ORDER BY id
		LIMIT $1 OFFSET $2
	) t
`

var GetCandidateByIDQuery string = `
	SELECT COALESCE(json_agg(t), '[]'::json)
	FROM hirevec.general.candidates t
	WHERE t.id = $1
`

var GetCandidatesQuery string = `
	SELECT COALESCE(json_agg(t), '[]'::json)
	FROM (
		SELECT *
		FROM hirevec.general.candidates
		ORDER BY id 
		LIMIT $1 OFFSET $2
	) t
`
