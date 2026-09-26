-- :name createUsersTable
CREATE TABLE IF NOT EXISTS users (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    email       TEXT NOT NULL UNIQUE,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    age         INT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
)

-- :name createProductsTable
CREATE TABLE IF NOT EXISTS products (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    price       DECIMAL(10,2) NOT NULL,
    stock       INT NOT NULL DEFAULT 0,
    category    VARCHAR(50) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
)

-- :name createOrdersTable
CREATE TABLE IF NOT EXISTS orders (
    id          SERIAL PRIMARY KEY,
    user_id     INT NOT NULL REFERENCES users(id),
    product_id  INT NOT NULL REFERENCES products(id),
    quantity    INT NOT NULL DEFAULT 1,
    total       DECIMAL(10,2) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
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
