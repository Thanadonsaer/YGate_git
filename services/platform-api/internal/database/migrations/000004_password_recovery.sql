CREATE TABLE password_recovery_attempt (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    operation text NOT NULL,
    identifier text NOT NULL DEFAULT '',
    source_ip inet,
    success boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT password_recovery_operation_valid CHECK (operation IN ('REQUEST', 'RESET'))
);

CREATE INDEX password_recovery_identifier_time_idx
    ON password_recovery_attempt (operation, identifier, created_at DESC);
CREATE INDEX password_recovery_ip_time_idx
    ON password_recovery_attempt (operation, source_ip, created_at DESC);
