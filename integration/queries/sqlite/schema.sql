-- :name createUsersTable
CREATE TABLE IF NOT EXISTS users (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    email       TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'active',
    age         INTEGER NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  TIMESTAMP
)

-- :name createProductsTable
CREATE TABLE IF NOT EXISTS products (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    price       DECIMAL(10,2) NOT NULL,
    stock       INTEGER NOT NULL DEFAULT 0,
    category    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  TIMESTAMP
)

-- :name createOrdersTable
CREATE TABLE IF NOT EXISTS orders (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    product_id  INTEGER NOT NULL REFERENCES products(id),
    quantity    INTEGER NOT NULL DEFAULT 1,
    total       DECIMAL(10,2) NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  TIMESTAMP
)

-- :name dropOrdersTable
DROP TABLE IF EXISTS orders

-- :name dropProductsTable
DROP TABLE IF EXISTS products

-- :name dropUsersTable
DROP TABLE IF EXISTS users

-- :name insertUser
INSERT INTO users (name, email, status, age) VALUES (?, ?, ?, ?) RETURNING id

-- :name insertProduct
INSERT INTO products (name, price, stock, category, status) VALUES (?, ?, ?, ?, ?) RETURNING id

-- :name insertOrder
INSERT INTO orders (user_id, product_id, quantity, total) VALUES (?, ?, ?, ?) RETURNING id
