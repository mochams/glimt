-- MySQL has no RETURNING: inserts report the new id through LastInsertId.

-- :name createUsersTable
CREATE TABLE IF NOT EXISTS users (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    email       VARCHAR(255) NOT NULL UNIQUE,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    age         INT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME NULL
)

-- :name createProductsTable
CREATE TABLE IF NOT EXISTS products (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    price       DECIMAL(10,2) NOT NULL,
    stock       INT NOT NULL DEFAULT 0,
    category    VARCHAR(50) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME NULL
)

-- :name createOrdersTable
CREATE TABLE IF NOT EXISTS orders (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    user_id     INT NOT NULL,
    product_id  INT NOT NULL,
    quantity    INT NOT NULL DEFAULT 1,
    total       DECIMAL(10,2) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME NULL,
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (product_id) REFERENCES products(id)
)

-- :name dropOrdersTable
DROP TABLE IF EXISTS orders

-- :name dropProductsTable
DROP TABLE IF EXISTS products

-- :name dropUsersTable
DROP TABLE IF EXISTS users

-- :name insertUser
INSERT INTO users (name, email, status, age) VALUES (?, ?, ?, ?)

-- :name insertProduct
INSERT INTO products (name, price, stock, category, status) VALUES (?, ?, ?, ?, ?)

-- :name insertOrder
INSERT INTO orders (user_id, product_id, quantity, total) VALUES (?, ?, ?, ?)
