-- Orders.

-- name: listOrders
-- Orders of an organization, newest first.
SELECT o.id, o.total, o.status
FROM orders o
WHERE o.org_id = :org
ORDER BY o.created_at DESC, o.id;

-- name: ordersByIDs
SELECT id, total FROM orders WHERE id IN (:ids) AND org_id = :org;

-- name: cancelOrder
UPDATE orders SET status = 'cancelled' WHERE id = :id AND org_id = :org RETURNING id;
