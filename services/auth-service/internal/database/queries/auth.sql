-- name: GetLoginUser :one
SELECT id, organization_id, email, display_name, password_hash, status,
       failed_login_count, locked_until
FROM auth.app_user
WHERE email = lower(sqlc.arg(identifier))
   OR username = lower(sqlc.arg(identifier))
LIMIT 1;

-- name: CountRecentFailedAuthAttempts :one
SELECT count(*)::bigint
FROM auth.auth_attempt
WHERE success = false
  AND created_at >= now() - interval '15 minutes'
  AND (identifier = lower(sqlc.arg(identifier)) OR source_ip = sqlc.arg(source_ip));

-- name: RecordAuthAttempt :exec
INSERT INTO auth.auth_attempt (identifier, source_ip, success)
VALUES (lower(sqlc.arg(identifier)), sqlc.arg(source_ip), sqlc.arg(success));

-- name: RecordLoginFailure :one
UPDATE auth.app_user
SET failed_login_count = failed_login_count + 1,
    locked_until = CASE
        WHEN failed_login_count + 1 >= 5 THEN now() + interval '15 minutes'
        ELSE locked_until
    END,
    updated_at = now()
WHERE id = sqlc.arg(user_id)
RETURNING failed_login_count, locked_until;

-- name: RecordLoginSuccess :exec
UPDATE auth.app_user
SET failed_login_count = 0,
    locked_until = NULL,
    updated_at = now()
WHERE id = sqlc.arg(user_id);

-- name: CreateUserSession :exec
INSERT INTO auth.user_session (
    id, organization_id, user_id, token_hash, csrf_hash, expires_at, idle_expires_at,
    client_ip, user_agent
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(csrf_hash),
    sqlc.arg(expires_at), sqlc.arg(idle_expires_at), sqlc.arg(client_ip), sqlc.arg(user_agent)
);

-- name: CreateAuditEvent :exec
INSERT INTO audit_log (
    organization_id, actor_user_id, action, target_type, target_id,
    after_data, source_ip, correlation_id
) VALUES (
    sqlc.arg(organization_id), sqlc.arg(actor_user_id), sqlc.arg(action),
    sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(after_data),
    sqlc.arg(source_ip), sqlc.arg(correlation_id)
);
-- name: GetActiveSession :one
SELECT s.id AS session_id, s.organization_id, s.user_id, s.csrf_hash,
       s.expires_at, s.idle_expires_at, u.email, u.display_name, u.password_hash
FROM auth.user_session s
JOIN auth.app_user u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg(token_hash)
  AND s.revoked_at IS NULL
  AND s.expires_at > now()
  AND s.idle_expires_at > now()
  AND u.status = 'ACTIVE'
LIMIT 1;

-- name: TouchSession :exec
UPDATE auth.user_session
SET last_seen_at = now(),
    idle_expires_at = LEAST(expires_at, now() + sqlc.arg(idle_seconds)::bigint * interval '1 second')
WHERE id = sqlc.arg(session_id)
  AND last_seen_at < now() - interval '1 minute';

-- name: RevokeSession :exec
UPDATE auth.user_session
SET revoked_at = COALESCE(revoked_at, now())
WHERE id = sqlc.arg(session_id);

-- name: RevokeAllUserSessions :exec
UPDATE auth.user_session
SET revoked_at = COALESCE(revoked_at, now())
WHERE user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL;

-- name: UpdatePasswordAndRevokeOtherSessions :exec
WITH changed AS (
    UPDATE auth.app_user AS u
    SET password_hash = sqlc.arg(password_hash),
        password_changed_at = now(),
        updated_at = now()
    WHERE u.id = sqlc.arg(user_id)
)
UPDATE auth.user_session AS s
SET revoked_at = COALESCE(s.revoked_at, now())
WHERE s.user_id = sqlc.arg(user_id)
  AND s.id <> sqlc.arg(current_session_id)
  AND s.revoked_at IS NULL;
-- name: GetPasswordResetUser :one
SELECT id, organization_id, email, status
FROM auth.app_user
WHERE email = lower(sqlc.arg(email))
LIMIT 1;

-- name: CountRecentPasswordRecoveryAttempts :one
SELECT count(*)::bigint
FROM auth.password_recovery_attempt
WHERE operation = sqlc.arg(operation)
  AND created_at >= now() - interval '15 minutes'
  AND (
      (sqlc.arg(identifier)::text <> '' AND identifier = lower(sqlc.arg(identifier)::text))
      OR (sqlc.narg(source_ip)::inet IS NOT NULL AND source_ip = sqlc.narg(source_ip)::inet)
  );

-- name: RecordPasswordRecoveryAttempt :exec
INSERT INTO auth.password_recovery_attempt (operation, identifier, source_ip, success)
VALUES (sqlc.arg(operation), lower(sqlc.arg(identifier)), sqlc.arg(source_ip), sqlc.arg(success));

-- name: InvalidatePasswordResetTokens :exec
UPDATE auth.password_reset_token
SET used_at = COALESCE(used_at, now())
WHERE user_id = sqlc.arg(user_id)
  AND used_at IS NULL;

-- name: CreatePasswordResetToken :exec
INSERT INTO auth.password_reset_token (
    id, organization_id, user_id, token_hash, expires_at, requested_ip
) VALUES (
    sqlc.arg(id), sqlc.arg(organization_id), sqlc.arg(user_id),
    sqlc.arg(token_hash), sqlc.arg(expires_at), sqlc.arg(requested_ip)
);

-- name: GetActivePasswordResetToken :one
SELECT t.id AS token_id, t.organization_id, t.user_id, u.email
FROM auth.password_reset_token t
JOIN auth.app_user u ON u.id = t.user_id
WHERE t.token_hash = sqlc.arg(token_hash)
  AND t.used_at IS NULL
  AND t.expires_at > now()
  AND u.status = 'ACTIVE'
LIMIT 1
FOR UPDATE OF t;

-- name: MarkPasswordResetTokenUsed :exec
UPDATE auth.password_reset_token
SET used_at = now()
WHERE id = sqlc.arg(token_id)
  AND used_at IS NULL;

-- name: UpdateUserPassword :exec
UPDATE auth.app_user
SET password_hash = sqlc.arg(password_hash),
    password_changed_at = now(),
    failed_login_count = 0,
    locked_until = NULL,
    updated_at = now()
WHERE id = sqlc.arg(user_id);

-- name: ListUserSessions :many
SELECT id, expires_at, idle_expires_at, last_seen_at, revoked_at,
       client_ip, user_agent, created_at
FROM auth.user_session
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC
LIMIT 100;

-- name: RevokeOwnedSession :one
UPDATE auth.user_session
SET revoked_at = COALESCE(revoked_at, now())
WHERE id = sqlc.arg(session_id)
  AND user_id = sqlc.arg(user_id)
RETURNING id;
