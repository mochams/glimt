-- Queries shared by every dialect.
-- Schema and inserts differ per database and live in the dialect directories.

-- :name listUsers
SELECT * FROM users

-- :name countUsersByStatus
-- Fixed filter in SQL, dynamic filters from the builder at the marker.
SELECT status, COUNT(*) AS total
FROM users
WHERE deleted_at IS NULL /* :and */
GROUP BY status

-- :name listProducts
SELECT * FROM products

-- :name updateProductStock
UPDATE products SET stock = ? WHERE id = ?

-- :name listOrders
SELECT * FROM orders

-- :name updateOrderStatus
UPDATE orders SET status = ? WHERE id = ?

-- :name softDeleteOrder
UPDATE orders SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?

-- :name orderUserIDs
-- Subquery for InQuery.
SELECT user_id FROM orders

-- :name userOrders
-- Subquery for Exists, correlated with the outer users query.
SELECT 1 FROM orders o WHERE o.user_id = users.id /* :and */
