package hirevec

type QueryTemplate string

var GetPositionQuery = func() string {

	return ` 
	select
			json_build_object(
					'position_id', json_agg(t.position_id),
					'title', json_agg(t.title),
					'description', json_agg(t.description),
					'company', json_agg(t.company)
			)
	from hirevec.general.positions as t 
	where position_id = $1;
	`
}
