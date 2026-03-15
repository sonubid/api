# Skill: database

## When to use this skill

Load this skill when:
- Creating or modifying database migration files
- Writing or updating `internal/db/db.go`
- Implementing a new repository under `internal/repository/`
- Designing a new table or altering an existing schema

---

## Database conventions

### Table naming

- Table names are always **singular**: `auction`, `bid`, `user`. Never plural (`auctions`, `bids`).
- Column names use `snake_case`.
- Join tables follow `<table_a>_<table_b>` in alphabetical order, both singular.

### Primary keys

| Table type | ID strategy | Column definition |
|---|---|---|
| CRUD resource (e.g. `auction`) | DB-generated UUID | `id UUID PRIMARY KEY DEFAULT gen_random_uuid()` |
| Event / derived record (e.g. `bid`) | Go-generated UUID (`uuid.NewString()`) | `id UUID PRIMARY KEY` (no default) |

The distinction: CRUD resources are created via API endpoints where the DB owns identity. Events are created at the application boundary (e.g. a WebSocket message) and travel a pipeline before persistence — Go generates the ID so it is available throughout.

### Column types

| Go type | PostgreSQL type |
|---|---|
| `string` (ID, text) | `TEXT` or `UUID` |
| `uint64` (amounts, counts) | `BIGINT` (signed; max ~9.2 × 10¹⁸ is sufficient for bid amounts) |
| `time.Time` | `TIMESTAMPTZ` (always store with timezone) |
| `bool` | `BOOLEAN` |

**Note on `uint64` ↔ `BIGINT`:** pgx scans `BIGINT` into `int64`. The repository layer is responsible for the `int64 → uint64` conversion. Document the assumption that values never exceed `math.MaxInt64`.

### Foreign keys

Always declare explicit `REFERENCES` constraints. Never rely on application-level consistency alone.

### Indexes

For creating a index, always ask the user if they want to create one or suggest them an index if needed. Name indexes as `idx_<table>_<column(s)>`.

```sql
CREATE INDEX idx_bid_auction_amount ON bid(auction_id, amount DESC);
```

---

## Migrations

### Tooling

Migrations are managed with the [`migrate` CLI](https://github.com/golang-migrate/migrate).

### Creating a migration

```bash
migrate create -ext sql -dir internal/db/migrations -seq <short_description>
```

This produces two files:
- `internal/db/migrations/<N>_<short_description>.up.sql`
- `internal/db/migrations/<N>_<short_description>.down.sql`

**Always fill in both files.** A migration without a down file is incomplete.

### File naming

- Use lowercase `snake_case` for the description: `create_auction_and_bid`, `add_status_index`, `alter_bid_add_note`.
- Descriptions describe what the migration does, not the ticket number.

### Down migrations

Down migrations must precisely undo the up migration and leave the schema in the state it was before the up ran.

```sql
-- up
CREATE TABLE bid ( ... );
CREATE INDEX idx_bid_auction_amount ON bid(auction_id, amount DESC);

-- down
DROP TABLE IF EXISTS bid;
-- The index is dropped automatically when the table is dropped.
```

### Applying migrations in Go

Migrations are embedded into the binary using `//go:embed` and applied at startup via `db.RunMigrations(dsn)` in `cmd/api/main.go`. The `iofs` source driver is used:

```go
//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations(dsn string) error {
    src, _ := iofs.New(migrationsFS, "migrations")
    m, _ := migrate.NewWithSourceInstance("iofs", src, dsn)
    defer m.Close()
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("db: run migrations: %w", err)
    }
    return nil
}
```

### DSN format

The DSN passed to both `RunMigrations` and `db.Connect` must use the `pgx5://` scheme for the migrate driver and the standard `postgres://` scheme for `pgxpool`:

```go
// For migrations (golang-migrate pgx/v5 driver):
migrateDSN := strings.Replace(dsn, "postgres://", "pgx5://", 1)

// For pgxpool:
pool, _ := pgxpool.New(ctx, dsn) // accepts postgres:// directly
```

Alternatively, expose a single `DATABASE_URL` env var and convert the scheme internally.

---

## Repository implementation

### Location

All repository implementations live in `internal/repository/`. One file per implementation:
- `memrepository.go` — in-memory (testing/MVP)
- `postgres.go` — PostgreSQL via `pgx`

### Interface compliance

Every implementation must declare a compile-time assertion:

```go
var _ auction.Repository = (*PostgresRepository)(nil)
```

### Connection pool

Repositories receive a `*pgxpool.Pool` at construction time. They never open or close connections themselves.

```go
type PostgresRepository struct {
    pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
    return &PostgresRepository{pool: pool}
}
```

### Query style

Use raw `pgx` queries — no ORM, no query builder. SQL is written inline. For non-trivial queries, define the SQL as a package-level constant.

```go
const sqlSaveBid = `
    INSERT INTO bid (id, auction_id, user_id, amount, placed_at)
    VALUES ($1, $2, $3, $4, $5)`
```

### Error wrapping

Wrap all database errors with package and operation context:

```go
if err != nil {
    return fmt.Errorf("repository: save bid: %w", err)
}
```

---

## Docker

### Location

All Docker-related files live in `/docker/` at the project root. Never inside `internal/` — Docker is infrastructure, not Go code.

### Compose file

```
docker/
  docker-compose.yml
```

### PostgreSQL image

Use `postgres:18.3-alpine3.23`. Always pin the full patch version and Alpine tag.

### Local dev DSN

```
postgres://sonubid:sonubid@localhost:5432/sonubid?sslmode=disable
```

---

## Environment variables

| Variable | Description | Example |
|---|---|---|
| `DATABASE_URL` | Full PostgreSQL DSN | `postgres://sonubid:sonubid@localhost:5432/sonubid?sslmode=disable` |
| `ALLOWED_ORIGIN` | WebSocket allowed origin | `http://localhost:3000` |

Read all config from environment variables at startup in `cmd/api/main.go`. No config files.
