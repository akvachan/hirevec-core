package hirevec

type QueryTemplate string

var GetPositionByID = ` 
select position_id, title, description, company
from hirevec.general.positions 
where position_id = $1;
`
