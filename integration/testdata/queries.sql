-- Real-world application queries for the oracle. Statements end with ";".

SELECT id, email, created_at FROM users WHERE org_id = :org AND deleted_at IS NULL ORDER BY created_at DESC LIMIT :limit OFFSET :offset;

SELECT o.id, o.total, u.email
FROM orders o
JOIN users u ON u.id = o.user_id
LEFT JOIN refunds r ON r.order_id = o.id
WHERE o.org_id = :org AND o.status IN (:statuses) AND r.id IS NULL
ORDER BY o.created_at DESC, o.id
LIMIT 50;

SELECT count(*) FROM orders WHERE org_id = :org AND created_at >= :since AND created_at < :until;

SELECT status, count(*) AS n, sum(total) AS revenue
FROM orders
WHERE org_id = :org
GROUP BY status
HAVING count(*) > :min
ORDER BY revenue DESC NULLS LAST;

SELECT id FROM jobs WHERE queue = :queue AND run_at <= now() ORDER BY priority DESC, id LIMIT :batch FOR UPDATE SKIP LOCKED;

UPDATE jobs SET state = 'running', attempts = attempts + 1, started_at = now() WHERE id = ANY(:ids) RETURNING id, payload;

INSERT INTO users (org_id, email, name) VALUES (:org, lower(:email), :name)
ON CONFLICT (org_id, email) DO UPDATE SET name = excluded.name, updated_at = now()
RETURNING id, (xmax = 0) AS inserted;

INSERT INTO events (org_id, kind, data) VALUES (:org, :kind, :data::jsonb);

INSERT INTO daily_totals (day, org_id, total)
SELECT date_trunc('day', created_at), org_id, sum(total)
FROM orders
WHERE created_at >= :since
GROUP BY 1, 2
ON CONFLICT (day, org_id) DO UPDATE SET total = excluded.total;

DELETE FROM sessions WHERE expires_at < now() - interval '30 days' RETURNING id;

DELETE FROM order_items oi USING orders o WHERE o.id = oi.order_id AND o.org_id = :org AND o.status = 'cancelled';

WITH recent AS (
    SELECT user_id, max(created_at) AS last_order
    FROM orders
    WHERE org_id = :org
    GROUP BY user_id
)
SELECT u.id, u.email, r.last_order
FROM users u
JOIN recent r ON r.user_id = u.id
WHERE r.last_order < now() - (:days || ' days')::interval;

WITH RECURSIVE tree AS (
    SELECT id, parent_id, name, 1 AS depth FROM categories WHERE id = :root
    UNION ALL
    SELECT c.id, c.parent_id, c.name, t.depth + 1 FROM categories c JOIN tree t ON c.parent_id = t.id
)
SELECT * FROM tree ORDER BY depth, name;

SELECT id, name, ts_rank(search, query) AS rank
FROM products, plainto_tsquery('english', :q) query
WHERE search @@ query AND price BETWEEN :min_price AND :max_price
ORDER BY rank DESC
LIMIT 20;

SELECT id FROM products WHERE attributes @> :attrs::jsonb AND attributes ? 'color' AND tags && :tags::text[];

SELECT id, data->>'name' AS name, data #>> '{address,city}' AS city FROM customers WHERE data->'flags' ? :flag;

SELECT DISTINCT ON (user_id) user_id, id, created_at FROM orders WHERE org_id = :org ORDER BY user_id, created_at DESC;

SELECT u.id, coalesce(sum(o.total) FILTER (WHERE o.status = 'paid'), 0) AS paid,
       row_number() OVER (PARTITION BY u.org_id ORDER BY u.created_at) AS seq
FROM users u
LEFT JOIN orders o ON o.user_id = u.id
WHERE u.org_id = :org
GROUP BY u.id;

SELECT id, CASE WHEN total > :big THEN 'large' WHEN total > :medium THEN 'medium' ELSE 'small' END AS size FROM orders;

SELECT * FROM orders WHERE (org_id, status) = (:org, :status) AND NOT (archived OR deleted_at IS NOT NULL);

SELECT id FROM users WHERE email ILIKE :pattern ESCAPE '\' AND id NOT IN (SELECT user_id FROM bans WHERE active);

SELECT t.offset, t.limit, t.order FROM settings t WHERE t.user = :user_id;

SELECT id, tags[1] AS first_tag, tags[2:3] AS some_tags, tags[lo:hi] FROM posts WHERE id = :id;

UPDATE accounts SET balance = balance - :amount, version = version + 1 WHERE id = :id AND version = :version AND balance >= :amount;

UPDATE users u SET plan = p.name FROM plans p WHERE p.id = u.plan_id AND u.org_id = :org;

UPDATE profiles SET settings['theme'] = to_jsonb(:theme::text), address.city = :city WHERE user_id = :user_id;

MERGE INTO inventory i
USING (SELECT :sku AS sku, :qty::int AS qty) s ON i.sku = s.sku
WHEN MATCHED AND i.qty + s.qty <= 0 THEN DELETE
WHEN MATCHED THEN UPDATE SET qty = i.qty + s.qty
WHEN NOT MATCHED THEN INSERT (sku, qty) VALUES (s.sku, s.qty);

SELECT 1 FROM users WHERE id = :id FOR SHARE;

SELECT a.id FROM accounts a WHERE a.id = :id FOR NO KEY UPDATE OF a NOWAIT;

SELECT id FROM events ORDER BY id OFFSET :skip ROWS FETCH FIRST :take ROWS ONLY;

SELECT 'it''s', E'line\nbreak', $$dollar 'quoted'$$, $tag$ tagged $tag$, 'multi'
  'line';

TABLE plans;

VALUES (1, 'a'), (2, 'b') ORDER BY 1;

SELECT x FROM generate_series(1, 10) x WHERE x % 2 = 0 EXCEPT SELECT 4 ORDER BY 1;

SELECT count(*) rows FROM orders WHERE org_id = :org;

SELECT o.status, count(*) rows FROM orders o GROUP BY o.status;

SELECT r.n FROM ROWS FROM (generate_series(1, 3)) AS r(n);

SELECT * FROM orders o, ROWS FROM (generate_series(1, o.qty)) g WHERE o.id = :id;

SELECT * FROM orders o JOIN LATERAL ROWS FROM (generate_series(1, o.qty)) g ON true;

SELECT o.id FROM orders o JOIN customers c ON conflict = c.id WHERE o.org_id = :org;

UPDATE orders o SET status = 'x' FROM customers c JOIN regions r ON conflict = r.id WHERE o.customer_id = c.id;

DELETE FROM orders o USING customers c JOIN regions r ON conflict = r.id WHERE o.customer_id = c.id;
