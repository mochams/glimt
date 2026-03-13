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

-- :name insertOrder
INSERT INTO orders (user_id, product_id, quantity, total)
VALUES (?, ?, ?, ?)
RETURNING id

-- :name listOrders
SELECT * FROM orders

-- :name updateOrderStatus
UPDATE orders SET status = ? WHERE id = ?

-- :name softDeleteOrder
UPDATE orders SET deleted_at = NOW() WHERE id = ?
