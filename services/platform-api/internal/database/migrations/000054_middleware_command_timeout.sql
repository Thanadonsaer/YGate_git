ALTER TABLE auth.middleware_client
    ADD COLUMN command_timeout_seconds integer NOT NULL DEFAULT 60,
    ADD CONSTRAINT middleware_client_command_timeout_range
        CHECK (command_timeout_seconds BETWEEN 5 AND 300);
