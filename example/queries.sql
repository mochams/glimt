-- :name createUsersTable
CREATE TABLE IF NOT EXISTS users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL UNIQUE,
    age        INTEGER NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME
)

-- :name listUsers
SELECT * FROM users

-- :name insertUser
INSERT INTO users (name, email, age, status)
VALUES (?, ?, ?, ?)

-- :name softDeleteUser
UPDATE users SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?
