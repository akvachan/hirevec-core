## Tasks tracking system
In order to create a task:
1. Go to a place in code where something needs to be done and type: `\\ TODO ./todos/DDMMYY-hhmmss-Title.md`.
2. `cp` from `./todos/templates/Todo.md` to `./todos/DDMMYY-hhmmss-Title.md`
3. `open ./todos/DDMMYY-hhmmss-Title.md`
4. Fill out info in `{{}}`
5. After task is done, set `status: Done` and remove the comment from the code.

## Installation
- Development setup with a hot-reload:
```
make watch
```

## Guidelines
- Never use `text/template` for values insertion, only for conditional structure inclusion. For example:

Do this:
```sql
select
        json_build_object(
                'position_id', json_agg(t.position_id),
                'title', json_agg(t.title),
                'description', json_agg(t.description),
                'company', json_agg(t.company)
        )
from hirevec.general.positions as t 

where position_id = $1;
```

Or this:
```sql
select
        json_build_object(
                'position_id', json_agg(t.position_id),
                'title', json_agg(t.title),
                'description', json_agg(t.description),
                'company', json_agg(t.company)
        )
from hirevec.general.positions as t 

{{if position_id}}
where position_id = $1;
{{end}}
```

**DO NOT** do this:
```sql
select
        json_build_object(
                'position_id', json_agg(t.position_id),
                'title', json_agg(t.title),
                'description', json_agg(t.description),
                'company', json_agg(t.company)
        )
from hirevec.general.positions as t 

where position_id = {{position_id}};
```

- Never use closers, interfaces or contexts unless there is no other way to do what needs to be done.
- Do not download, install, use 3rd party dependencies beside those that are already available in the [go.mod].
