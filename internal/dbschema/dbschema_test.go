package dbschema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSplitStatements(t *testing.T) {
	sql := `-- comment; not a statement
CREATE TABLE a (id int); /* block; comment */
INSERT INTO a VALUES ('x;y');
CREATE FUNCTION f() RETURNS trigger AS $$ BEGIN NEW.x := 1; RETURN NEW; END; $$ LANGUAGE plpgsql;
CREATE FUNCTION g() RETURNS int AS $body$ SELECT 1; $body$ LANGUAGE sql;
SELECT 'it''s; fine'`
	got := SplitStatements(sql)
	if len(got) != 5 {
		t.Fatalf("got %d statements: %q", len(got), got)
	}
	if !strings.Contains(got[1], "'x;y'") || !strings.Contains(got[2], "RETURN NEW; END;") || !strings.Contains(got[4], "it''s; fine") {
		t.Errorf("quotes or dollar bodies split wrongly: %q", got)
	}
}

func TestGolangMigrateReplay(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"000001_init.up.sql": `
CREATE TYPE order_status AS ENUM ('pending', 'paid', 'cancelled');
CREATE TABLE IF NOT EXISTS public.orders (
  id BIGSERIAL PRIMARY KEY,
  public_id TEXT NOT NULL UNIQUE,
  user_id BIGINT NOT NULL,
  status order_status NOT NULL DEFAULT 'pending',
  total_amount NUMERIC(12, 2) NOT NULL CHECK (total_amount >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE order_items (
  id bigserial,
  order_id bigint NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  sku text NOT NULL,
  quantity int NOT NULL,
  CONSTRAINT order_items_pkey PRIMARY KEY (id),
  CONSTRAINT qty_positive CHECK (quantity > 0)
);
CREATE INDEX idx_orders_user_created ON orders (user_id, created_at DESC);`,
		"000001_init.down.sql":   "DROP TABLE order_items; DROP TABLE orders;",
		"000002_payments.up.sql": "ALTER TABLE orders ADD COLUMN payment_id text, ADD COLUMN IF NOT EXISTS note text;\nALTER TYPE order_status ADD VALUE 'refunded' AFTER 'paid';",
		"000003_cleanup.up.sql":  "ALTER TABLE orders DROP COLUMN note;\nALTER TABLE orders RENAME COLUMN total_amount TO total;\nALTER TABLE orders ALTER COLUMN total TYPE bigint USING (total*100)::bigint;\nALTER TABLE orders ALTER COLUMN payment_id SET NOT NULL;\nCREATE UNIQUE INDEX CONCURRENTLY uq_orders_payment ON orders (payment_id) WHERE payment_id IS NOT NULL;\nUPDATE orders SET total = 0 WHERE total IS NULL;",
		"000010_rename.up.sql":   "ALTER TABLE order_items RENAME TO order_lines;",
		"000004_temp.up.sql":     "CREATE TABLE scratch (x int); DROP TABLE IF EXISTS scratch;",
		"000005_function.up.sql": "CREATE OR REPLACE FUNCTION touch() RETURNS trigger AS $$ BEGIN NEW.updated_at = now(); RETURN NEW; END; $$ LANGUAGE plpgsql;",
		"000006_weird.up.sql":    "CREATE POLICY p ON orders USING (true); FROBNICATE orders;",
		"README.md":              "not sql",
	})
	s, err := Load(dir, "orders-svc/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if s.Tool != "golang-migrate" {
		t.Errorf("tool = %s", s.Tool)
	}
	if got := strings.Join(s.TableOrder, ","); got != "orders,order_lines" {
		t.Errorf("tables = %s", got)
	}
	o := s.Tables["orders"]
	byName := map[string]*Column{}
	for _, c := range o.Columns {
		byName[c.Name] = c
	}
	if byName["note"] != nil {
		t.Error("dropped column still present")
	}
	if c := byName["total"]; c == nil || c.Type != "bigint" {
		t.Errorf("renamed/retyped column wrong: %+v", c)
	}
	if c := byName["payment_id"]; c == nil || c.Nullable {
		t.Errorf("payment_id should be NOT NULL: %+v", c)
	}
	if c := byName["status"]; c.Default != "'pending'" || c.Nullable {
		t.Errorf("status default/null wrong: %+v", c)
	}
	if !byName["id"].PK || !byName["public_id"].Unique {
		t.Error("pk/unique not detected")
	}
	if got := strings.Join(s.Enums["order_status"], ","); got != "pending,paid,refunded,cancelled" {
		t.Errorf("enum = %s", got)
	}
	if len(s.FKs) != 1 || s.FKs[0].Table != "order_lines" || s.FKs[0].RefTable != "orders" || s.FKs[0].OnDelete != "cascade" {
		t.Errorf("fk after table rename: %+v", s.FKs)
	}
	if ix := s.Indexes["uq_orders_payment"]; ix == nil || !ix.Unique || ix.Where == "" {
		t.Errorf("partial unique index: %+v", ix)
	}
	if ix := s.Indexes["idx_orders_user_created"]; ix == nil || strings.Join(ix.Columns, ",") != "user_id,created_at DESC" {
		t.Errorf("index columns: %+v", ix)
	}
	if len(s.Skipped) != 1 || !strings.Contains(s.Skipped[0], "FROBNICATE") {
		t.Errorf("skipped = %v", s.Skipped)
	}
	if s.Migrations[len(s.Migrations)-1].Version != "000010" {
		t.Error("migrations must sort numerically (000010 after 000006)")
	}

	md := s.Render("2026-11-20")
	for _, want := range []string{GenStart, GenEnd, "### `orders`", "| `payment_id` | `text` | no |", "FK → `orders.id`, on delete cascade", "`order_lines.order_id` → `orders.id`", "| `order_status` (type) | pending, paid, refunded, cancelled |", "unique `uq_orders_payment (payment_id)` where payment_id IS NOT NULL", "Check: `quantity > 0`", "Not understood, check by hand"} {
		if !strings.Contains(md, want) {
			t.Errorf("render missing %q", want)
		}
	}
	if strings.Index(md, "`000010_rename`") > strings.Index(md, "`000001_init`") {
		t.Error("migration log must be newest first")
	}
}

func TestGooseAndChecks(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"20261003120000_init.sql": `-- +goose Up
-- +goose StatementBegin
CREATE TABLE charges (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
  amount bigint NOT NULL
);
-- +goose StatementEnd
-- +goose Down
DROP TABLE charges;`,
		"20261021090000_refunds.sql": "-- +goose Up\nCREATE TABLE refunds (id bigserial PRIMARY KEY, charge_id bigint NOT NULL, kind text, CONSTRAINT fk_charge FOREIGN KEY (charge_id) REFERENCES charges (id), CONSTRAINT kind_ok CHECK (kind = ANY (ARRAY['full'::text, 'partial'::text])));\n-- +goose Down\nDROP TABLE refunds;",
	})
	s, err := Load(dir, "payments-svc/db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	if s.Tool != "goose" || len(s.Tables) != 2 {
		t.Fatalf("tool=%s tables=%v", s.Tool, s.TableOrder)
	}
	if s.Migrations[0].Date != "2026-10-03" {
		t.Errorf("date from version = %q", s.Migrations[0].Date)
	}
	md := s.Render("2026-11-20")
	for _, want := range []string{"`charges.status` (check) | pending, succeeded, failed", "`refunds.kind` (check) | full, partial", "auto-increment", "`refunds.charge_id` → `charges.id`"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "DROP TABLE") || len(s.Skipped) != 0 {
		t.Errorf("goose Down section must be ignored; skipped=%v", s.Skipped)
	}
}

func TestMySQLStyle(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"001_users.sql":  "CREATE TABLE `users` (`id` INT NOT NULL AUTO_INCREMENT, `email` VARCHAR(255) NOT NULL, PRIMARY KEY (`id`), UNIQUE KEY `uq_email` (`email`), KEY `idx_email` (`email`)) ENGINE=InnoDB;",
		"002_change.sql": "ALTER TABLE users MODIFY COLUMN email VARCHAR(320) NOT NULL, CHANGE COLUMN id user_id INT NOT NULL AUTO_INCREMENT;\nRENAME TABLE users TO accounts;",
	})
	s, err := Load(dir, "users-svc/migrations")
	if err != nil {
		t.Fatal(err)
	}
	a := s.Tables["accounts"]
	if a == nil {
		t.Fatalf("rename table failed: %v", s.TableOrder)
	}
	if c := a.col("email"); c == nil || c.Type != "varchar(320)" {
		t.Errorf("modify column: %+v", c)
	}
	if a.col("user_id") == nil || a.col("id") != nil {
		t.Error("change column rename failed")
	}
}

func TestPrisma(t *testing.T) {
	src := `
datasource db { provider = "postgresql" url = env("DATABASE_URL") }
enum Role {
  USER
  ADMIN
}
model User {
  id        Int      @id @default(autoincrement())
  email     String   @unique @db.VarChar(320)
  role      Role     @default(USER)
  posts     Post[]
  createdAt DateTime @default(now()) @map("created_at")
  @@map("users")
}
model Post {
  id       Int    @id @default(autoincrement())
  title    String
  authorId Int    @map("author_id")
  author   User   @relation(fields: [authorId], references: [id], onDelete: Cascade)
  @@index([authorId])
}`
	s := ParsePrisma(src, "api/prisma/schema.prisma")
	if strings.Join(s.TableOrder, ",") != "users,Post" {
		t.Fatalf("tables = %v", s.TableOrder)
	}
	u := s.Tables["users"]
	if c := u.col("email"); c == nil || c.Type != "varchar(320)" || !c.Unique {
		t.Errorf("email: %+v", c)
	}
	if u.col("created_at") == nil || u.col("posts") != nil {
		t.Error("@map or relation-list handling wrong")
	}
	if len(s.FKs) != 1 || s.FKs[0].Column != "author_id" || s.FKs[0].RefTable != "users" || s.FKs[0].OnDelete != "cascade" {
		t.Errorf("fk: %+v", s.FKs)
	}
	if strings.Join(s.Enums["Role"], ",") != "USER,ADMIN" {
		t.Errorf("enum: %v", s.Enums)
	}
	if len(s.IndexOrder) != 1 || s.Indexes[s.IndexOrder[0]].Columns[0] != "author_id" {
		t.Errorf("index: %v", s.Indexes)
	}
}

func TestDetect(t *testing.T) {
	repo := writeFiles(t, map[string]string{"db/migrations/001_a.up.sql": "CREATE TABLE a (id int);", "main.go": "package main"})
	if got, err := Detect(repo); err != nil || got != "db/migrations" {
		t.Errorf("Detect = %q, %v", got, err)
	}
	prisma := writeFiles(t, map[string]string{"prisma/schema.prisma": "model A { id Int @id }"})
	if got, err := Detect(prisma); err != nil || got != "prisma/schema.prisma" {
		t.Errorf("Detect prisma = %q, %v", got, err)
	}
	if _, err := Detect(t.TempDir()); err == nil {
		t.Error("empty repo should report no migrations")
	}
}

func TestFlyway(t *testing.T) {
	repo := writeFiles(t, map[string]string{
		"src/main/resources/db/migration/V1__init.sql":         "CREATE TABLE accounts (id BIGINT PRIMARY KEY, email VARCHAR(255) NOT NULL);",
		"src/main/resources/db/migration/V1.1__add_name.sql":   "ALTER TABLE accounts ADD COLUMN name VARCHAR(100);",
		"src/main/resources/db/migration/V1.10__add_phone.sql": "ALTER TABLE accounts ADD COLUMN phone VARCHAR(20);",
		"src/main/resources/db/migration/V1.2__rename.sql":     "ALTER TABLE accounts RENAME COLUMN name TO full_name;",
		"src/main/resources/db/migration/V2__orders.sql":       "CREATE TABLE orders (id BIGINT PRIMARY KEY, account_id BIGINT REFERENCES accounts(id));",
		"src/main/resources/db/migration/R__views.sql":         "CREATE OR REPLACE VIEW active_accounts AS SELECT * FROM accounts;",
		"src/main/resources/db/migration/U2__undo_orders.sql":  "DROP TABLE orders;",
	})
	dir, err := Detect(repo)
	if err != nil || dir != "src/main/resources/db/migration" {
		t.Fatalf("Detect = %q, %v", dir, err)
	}
	s, err := Load(filepath.Join(repo, dir), "accounts-svc/"+dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Tool != "flyway" {
		t.Errorf("tool = %s", s.Tool)
	}
	var order []string
	for _, m := range s.Migrations {
		order = append(order, m.File)
	}
	want := "V1__init.sql,V1.1__add_name.sql,V1.2__rename.sql,V1.10__add_phone.sql,V2__orders.sql,R__views.sql"
	if got := strings.Join(order, ","); got != want {
		t.Errorf("order:\n got %s\nwant %s", got, want)
	}
	a := s.Tables["accounts"]
	if a.col("full_name") == nil || a.col("name") != nil || a.col("phone") == nil {
		t.Error("versions 1.1 < 1.2 < 1.10 must apply in numeric order")
	}
	if s.Tables["orders"] == nil {
		t.Error("undo (U) migrations must be ignored")
	}
}
