ALTER TABLE user_session ADD COLUMN csrf_hash bytea;

DELETE FROM user_session;

ALTER TABLE user_session ALTER COLUMN csrf_hash SET NOT NULL;
