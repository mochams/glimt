-- name: getUser
SELECT id, email FROM users WHERE id = :id;

-- name: createUsersTable
CREATE TABLE IF NOT EXISTS users (id bigserial PRIMARY KEY, email text NOT NULL);
