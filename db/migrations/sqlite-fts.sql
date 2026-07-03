begin transaction;

-- Positions FTS Setup
create virtual table if not exists positions_fts using fts5(
  id unindexed,
  title,
  description,
  company
);

create trigger if not exists positions_ai after insert on positions
begin
  insert into positions_fts(id, title, description, company)
  values (new.id, new.title, new.description, new.company);
end;

create trigger if not exists positions_ad after delete on positions
begin
  delete from positions_fts where id = old.id;
end;

create trigger if not exists positions_au after update on positions
begin
  delete from positions_fts where id = old.id;
  insert into positions_fts(id, title, description, company)
  values (new.id, new.title, new.description, new.company);
end;

-- Candidates FTS Setup
create virtual table if not exists candidates_fts using fts5(
  id unindexed,
  about
);

create trigger if not exists candidates_ai after insert on candidates
begin
  insert into candidates_fts(id, about)
  values (new.id, new.about);
end;

create trigger if not exists candidates_ad after delete on candidates
begin
  delete from candidates_fts where id = old.id;
end;

create trigger if not exists candidates_au after update on candidates
begin
  delete from candidates_fts where id = old.id;
  insert into candidates_fts(id, about)
  values (new.id, new.about);
end;

commit;
