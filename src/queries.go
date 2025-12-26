package hirevec

type QueryTemplate string

var GetPositionByIDQuery = ` 
select position_id, title, description, company
from hirevec.general.positions 
where position_id = $1;
`
