-- :name createUsersTable
CREATE TABLE IF NOT EXISTS users (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    email       TEXT NOT NULL UNIQUE,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    age         INT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

-- :name dropUsersTable
DROP TABLE IF EXISTS users

-- :name insertUser
INSERT INTO users (name, email, status, age)
VALUES (?, ?, ?, ?)
RETURNING id

-- :name listUsers
SELECT * FROM users
