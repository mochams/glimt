# glimt example

A minimal REST API demonstrating how to use [glimt](https://github.com/mochams/glimt) for dynamic SQL filtering in a real application.

The example uses SQLite — no database setup required. Clone and run.

## Run

```bash
git clone https://github.com/mochams/glimt
cd glimt/example

go run .
```

Server starts on `http://localhost:8080`.

## Flags

| Flag       | Default       | Description          |
| ---------- | ------------- | -------------------- |
| `-addr`    | `:8080`       | HTTP server address  |
| `-dsn`     | `glimt.db`    | SQLite database file |
| `-queries` | `queries.sql` | SQL queries file     |

```bash
# custom port and in-memory database
go run . -addr :9090 -dsn ":memory:"
```

## Endpoints

### List users

```txt
GET /users
```

All query parameters are optional:

| Parameter | Type   | Description                             |
| --------- | ------ | --------------------------------------- |
| `status`  | string | Filter by status (`active`, `inactive`) |
| `min_age` | int    | Filter by minimum age                   |
| `max_age` | int    | Filter by maximum age                   |
| `search`  | string | Filter by name (partial match)          |
| `limit`   | int    | Results per page (default 20)           |
| `offset`  | int    | Results to skip (default 0)             |

```bash
# all users
curl http://localhost:8080/users

# active users aged 18-65
curl "http://localhost:8080/users?status=active&min_age=18&max_age=65"

# search by name with pagination
curl "http://localhost:8080/users?search=alice&limit=10&offset=0"
```

### Get user

```txt
GET /users/{id}
```

```bash
curl http://localhost:8080/users/1
```

### Create user

```txt
POST /users
```

```bash
curl -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"name": "Alice", "email": "alice@example.com", "age": 30, "status": "active"}'
```

### Delete user

```txt
DELETE /users/{id}
```

Soft deletes the user — the record is retained in the database with `deleted_at` set.
Soft deleted users are excluded from all list and get responses.

```bash
curl -X DELETE http://localhost:8080/users/1
```

## Responses

All endpoints return JSON. Successful responses:

```json
// single resource
{ "data": { "id": 1, "name": "Alice", ... } }

// list
{ "data": [...], "count": 3 }
```

Error responses:

```json
// not found
{ "error": "user with id 1 not found" }

// validation failed
{
  "error": "validation failed",
  "data": [
    { "field": "email", "message": "is required" }
  ]
}
```

## How glimt is used

The example is built around glimt's core pattern — write SQL once in a file, filter dynamically at runtime:

**`queries.sql`** defines the base query:

```sql
-- :name listUsers
SELECT * FROM users
```

**`repository.go`** extends it based on request parameters:

```go
q := registry.MustGet("listUsers").
    Where(glimt.IsNull("deleted_at"))

if req.Status != "" {
    q.Where(glimt.Eq("status", req.Status))
}
if req.MinAge > 0 {
    q.Where(glimt.Gte("age", req.MinAge))
}
if req.Search != "" {
    q.Where(glimt.Like("name", "%" + req.Search + "%"))
}

sql, args := q.Limit(req.Limit).Offset(req.Offset).Build()
```

One base query. Any combination of filters. No string concatenation. No SQL injection risk.
