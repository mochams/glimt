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

-- :name dropProductsTable
DROP TABLE IF EXISTS products

-- :name insertProduct
INSERT INTO products (name, price, stock, category, status)
VALUES (?, ?, ?, ?, ?)
RETURNING id

-- :name listProducts
SELECT * FROM products

-- :name updateProductStock
UPDATE products SET stock = ? WHERE id = ?
