create or replace function insert_position() returns trigger as $$
declare
data json;
    notification json;
begin
select row_to_json(row) into notification from (select * from positions) row;
perform pg_notify('notify_chat', notification::text);
return null;
end;
$$ language plpgsql;

create trigger insert_position_notify after insert or update on positions for each row execute procedure insert_position();