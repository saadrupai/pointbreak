package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const ordersInit = `CREATE TABLE orders (id bigserial PRIMARY KEY, status text NOT NULL DEFAULT 'pending', user_id bigint NOT NULL);
CREATE TABLE order_items (id bigserial PRIMARY KEY, order_id bigint NOT NULL REFERENCES orders(id));`

// dbWS: three services, orders-svc with its own migrations, a shared db-migrations folder.
func dbWS(t *testing.T) *Workspace {
	t.Helper()
	ws := newWS(t, "2026-11-20 10:00", "orders-svc", "payments-svc", "notifications-svc", "db-migrations")
	if _, err := ws.Init([]string{"orders-svc", "payments-svc", "notifications-svc"}); err != nil {
		t.Fatal(err)
	}
	put(t, ws.Root, "orders-svc/migrations/000001_init.up.sql", ordersInit)
	put(t, ws.Root, "db-migrations/migrations/000001_core.up.sql", `CREATE TABLE customers (id bigserial PRIMARY KEY, email text NOT NULL UNIQUE);
CREATE TABLE charges (id bigserial PRIMARY KEY, customer_id bigint REFERENCES customers(id), amount bigint NOT NULL);
CREATE TABLE audit_events (id bigserial PRIMARY KEY, kind text);`)
	return ws
}

func TestSyncOwnDatabaseFromTemplate(t *testing.T) {
	ws := dbWS(t)
	msg, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "2 tables") || !strings.Contains(msg, "orders-svc/migrations") {
		t.Errorf("msg: %s", msg)
	}
	doc := read(t, ws, "orders-svc/database.md")
	for _, want := range []string{"# orders-svc — Database", "## Engine", "- Migrations: golang-migrate, `orders-svc/migrations`", "### `orders`", "## Data notes", "## Data owned elsewhere"} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "<table_name>") {
		t.Error("template placeholders should be gone")
	}
}

func TestSyncIsIdempotentAndKeepsHumanText(t *testing.T) {
	ws := dbWS(t)
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"}); err != nil {
		t.Fatal(err)
	}
	first := read(t, ws, "orders-svc/database.md")
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"}); err != nil {
		t.Fatal(err)
	}
	if second := read(t, ws, "orders-svc/database.md"); second != first {
		t.Error("syncing twice with the same migrations must give the same file")
	}

	edited := strings.Replace(first, "- (fill in: money units, PII columns, hot tables, retention)", "- Money is in cents. Never floats.", 1)
	put(t, ws.Root, "context/orders-svc/database.md", edited)
	put(t, ws.Root, "orders-svc/migrations/000002_refunds.up.sql", "ALTER TABLE orders ADD COLUMN refunded_at timestamptz;")
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"}); err != nil {
		t.Fatal(err)
	}
	doc := read(t, ws, "orders-svc/database.md")
	if !strings.Contains(doc, "Money is in cents. Never floats.") {
		t.Error("human Data notes were lost")
	}
	if !strings.Contains(doc, "`refunded_at`") || !strings.Contains(doc, "`000002_refunds`") {
		t.Errorf("new migration not reflected:\n%s", doc)
	}
	if strings.Count(doc, "### `orders`") != 1 {
		t.Error("generated block duplicated")
	}
}

func TestSyncHandWrittenDocNeedsOptIn(t *testing.T) {
	ws := dbWS(t)
	hand := "# orders-svc — Database\n\n## Engine\n- Postgres 16, sqlc\n\n## Tables\nold table notes\n\n## Relations\nold\n\n## Data notes\n- keep me\n"
	put(t, ws.Root, "context/orders-svc/database.md", hand)
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"}); err == nil || !strings.Contains(err.Error(), "replace_sections") {
		t.Fatalf("hand-written doc must not be overwritten silently: %v", err)
	}
	if read(t, ws, "context/../context/orders-svc/database.md"[len("context/"):]) != hand {
		t.Error("file changed despite the error")
	}
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc", ReplaceSections: true}); err != nil {
		t.Fatal(err)
	}
	doc := read(t, ws, "orders-svc/database.md")
	if !strings.Contains(doc, "- Postgres 16, sqlc") || !strings.Contains(doc, "- keep me") {
		t.Error("non-generated sections must be kept")
	}
	if strings.Contains(doc, "old table notes") || !strings.Contains(doc, "### `order_items`") {
		t.Errorf("generated sections not replaced:\n%s", doc)
	}
	if strings.Index(doc, "## Engine") > strings.Index(doc, "### `orders`") || strings.Index(doc, "### `orders`") > strings.Index(doc, "## Data notes") {
		t.Error("generated block should sit where the old Tables section was")
	}
}

func TestSyncDryRunWritesNothing(t *testing.T) {
	ws := dbWS(t)
	before := read(t, ws, "orders-svc/database.md")
	out, err := ws.SyncDatabase(SyncInput{Service: "orders-svc", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Dry run") || !strings.Contains(out, "### `orders`") {
		t.Errorf("dry run output: %s", out)
	}
	if read(t, ws, "orders-svc/database.md") != before {
		t.Error("dry run wrote the file")
	}
}

func TestSyncBrokenMarkersRefused(t *testing.T) {
	ws := dbWS(t)
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"}); err != nil {
		t.Fatal(err)
	}
	doc := read(t, ws, "orders-svc/database.md")
	broken := strings.Replace(doc, "<!-- pointbreak:generated:end -->", "", 1)
	put(t, ws.Root, "context/orders-svc/database.md", broken)
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"}); err == nil || !strings.Contains(err.Error(), "broken generated block") {
		t.Fatalf("want broken-marker error, got %v", err)
	}
	if read(t, ws, "orders-svc/database.md") != broken {
		t.Error("broken doc was modified")
	}
}

func TestSyncInputErrors(t *testing.T) {
	ws := dbWS(t)
	cases := []struct {
		name string
		in   SyncInput
		want string
	}{
		{"nothing", SyncInput{}, "pass service"},
		{"unknown service", SyncInput{Service: "ghost-svc"}, "init_workspace"},
		{"traversal service", SyncInput{Service: "../x"}, "invalid service name"},
		{"no migrations", SyncInput{Service: "payments-svc"}, "no migrations found"},
		{"source escapes", SyncInput{Service: "orders-svc", Source: "../../etc"}, "inside the workspace"},
		{"absolute source", SyncInput{Service: "orders-svc", Source: "/etc"}, "inside the workspace"},
		{"missing source", SyncInput{Service: "orders-svc", Source: "nope/migrations"}, "not found"},
		{"shared without source", SyncInput{Database: "core"}, "source is required"},
		{"bad db name", SyncInput{Database: "../core", Source: "db-migrations"}, "invalid database name"},
		{"shared first time no users", SyncInput{Database: "core", Source: "db-migrations"}, "users is required"},
		{"shared unknown user", SyncInput{Database: "core", Source: "db-migrations", Users: []string{"ghost"}}, "init_workspace"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ws.SyncDatabase(c.in)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestSyncExplicitSourceFindsNestedFolder(t *testing.T) {
	ws := dbWS(t)
	msg, err := ws.SyncDatabase(SyncInput{Service: "notifications-svc", Source: "orders-svc"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "orders-svc/migrations") {
		t.Errorf("should detect the migrations folder inside the given source: %s", msg)
	}
}

func TestSharedDatabaseForSomeServices(t *testing.T) {
	ws := dbWS(t)
	put(t, ws.Root, "payments-svc/internal/repo/charges.go", "package repo\nconst q = `SELECT id, amount FROM charges WHERE customer_id = $1`\nconst q2 = `INSERT INTO charges (amount) VALUES ($1)`\n")
	put(t, ws.Root, "payments-svc/internal/model/customer.go", "package model\nfunc (Customer) TableName() string { return \"customers\" }\n")
	put(t, ws.Root, "payments-svc/vendor/lib/x.go", "SELECT * FROM audit_events")                  // ignored: vendor
	put(t, ws.Root, "payments-svc/migrations/x.sql", "SELECT * FROM audit_events")                 // ignored: migrations
	put(t, ws.Root, "notifications-svc/main.go", "// we send emails to customers about charges\n") // plain words: not a reference

	msg, err := ws.SyncDatabase(SyncInput{Database: "core", Source: "db-migrations", Users: []string{"payments-svc", "notifications-svc"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "context/_db/core.md") || !strings.Contains(msg, "payments-svc, notifications-svc") {
		t.Errorf("msg: %s", msg)
	}
	shared := read(t, ws, "_db/core.md")
	for _, want := range []string{"# core — Shared Database", "## Used by", "| [payments-svc](../payments-svc/architecture.md) | `charges` (2), `customers` (1) |", "| [notifications-svc](../notifications-svc/architecture.md) | none found |", "### `audit_events`", "## Ownership"} {
		if !strings.Contains(shared, want) {
			t.Errorf("shared doc missing %q:\n%s", want, shared)
		}
	}
	pay := read(t, ws, "payments-svc/database.md")
	if !strings.Contains(pay, "uses the shared database `core`") || !strings.Contains(pay, "`charges` (2)") || !strings.Contains(pay, "## Tables this service owns") {
		t.Errorf("payments pointer:\n%s", pay)
	}
	if orders := read(t, ws, "orders-svc/database.md"); strings.Contains(orders, "shared database") {
		t.Error("orders-svc does not use the shared database")
	}
	if svcs, _ := ws.Services(); len(svcs) != 3 {
		t.Errorf("_db must never be listed as a service: %v", svcs)
	}

	// later syncs reuse the linked services and refresh, not duplicate, the pointer block
	put(t, ws.Root, "db-migrations/migrations/000002_refunds.up.sql", "CREATE TABLE refunds (id bigserial PRIMARY KEY, charge_id bigint REFERENCES charges(id));")
	put(t, ws.Root, "notifications-svc/internal/q.go", "const q = `SELECT * FROM refunds`")
	msg, err = ws.SyncDatabase(SyncInput{Database: "core", Source: "db-migrations"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "payments-svc") || !strings.Contains(msg, "notifications-svc") {
		t.Errorf("linked users not reused: %s", msg)
	}
	notif := read(t, ws, "notifications-svc/database.md")
	if strings.Count(notif, "## Shared database") != 1 || !strings.Contains(notif, "`refunds` (1)") {
		t.Errorf("pointer block not refreshed in place:\n%s", notif)
	}
	if strings.Count(read(t, ws, "_db/core.md"), "## Used by") != 1 {
		t.Error("users block duplicated")
	}
}

func TestSharedPointerKeepsHandWrittenServiceDoc(t *testing.T) {
	ws := dbWS(t)
	hand := "# payments-svc — Database\n\n## Data notes\n- payments writes charges only\n"
	put(t, ws.Root, "context/payments-svc/database.md", hand)
	if _, err := ws.SyncDatabase(SyncInput{Database: "core", Source: "db-migrations", Users: []string{"payments-svc"}}); err != nil {
		t.Fatal(err)
	}
	doc := read(t, ws, "payments-svc/database.md")
	if !strings.Contains(doc, "- payments writes charges only") || !strings.Contains(doc, "## Shared database") {
		t.Errorf("pointer must be added without losing text:\n%s", doc)
	}
	if strings.Index(doc, "## Shared database") > strings.Index(doc, "## Data notes") {
		t.Error("pointer block should go right after the title")
	}
}

func TestAllServicesShareOneDatabase(t *testing.T) {
	ws := dbWS(t)
	msg, err := ws.SyncDatabase(SyncInput{Database: "main", Source: "db-migrations", Users: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"orders-svc", "payments-svc", "notifications-svc"} {
		if !strings.Contains(msg, s) || !strings.Contains(read(t, ws, s+"/database.md"), "shared database `main`") {
			t.Errorf("%s not linked to main", s)
		}
	}
}

func TestLoadContextIncludesSharedDocOnce(t *testing.T) {
	ws := dbWS(t)
	if _, err := ws.SyncDatabase(SyncInput{Database: "core", Source: "db-migrations", Users: []string{"payments-svc", "notifications-svc"}}); err != nil {
		t.Fatal(err)
	}
	out, err := ws.LoadContext(LoadOptions{Services: []string{"payments-svc", "notifications-svc", "payments-svc"}, SkipRules: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "<!-- context/_db/core.md -->"); n != 1 {
		t.Errorf("shared doc loaded %d times, want 1", n)
	}
	if n := strings.Count(out, "<!-- context/payments-svc/architecture.md -->"); n != 1 {
		t.Errorf("duplicate service loaded %d times, want 1", n)
	}
	if strings.Index(out, "<!-- context/_db/core.md -->") < strings.Index(out, "<!-- context/payments-svc/database.md -->") {
		t.Error("shared doc should follow the first service's database.md")
	}
	noDB, _ := ws.LoadContext(LoadOptions{Services: []string{"payments-svc"}, Files: []string{"temp.md"}})
	if strings.Contains(noDB, "_db/core.md") {
		t.Error("files filter without database.md must not load the shared doc")
	}
}

func TestImpactSearchesSharedDocs(t *testing.T) {
	ws := dbWS(t)
	if _, err := ws.SyncDatabase(SyncInput{Database: "core", Source: "db-migrations", Users: []string{"*"}}); err != nil {
		t.Fatal(err)
	}
	out, err := ws.Impact("customer_id")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "shared database core") {
		t.Errorf("impact missed the shared doc:\n%s", out)
	}
}

func TestMixedOwnAndSharedDatabases(t *testing.T) {
	ws := dbWS(t)
	if _, err := ws.SyncDatabase(SyncInput{Service: "orders-svc"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.SyncDatabase(SyncInput{Database: "core", Source: "db-migrations", Users: []string{"payments-svc", "notifications-svc"}}); err != nil {
		t.Fatal(err)
	}
	orders := read(t, ws, "orders-svc/database.md")
	if !strings.Contains(orders, "### `orders`") || strings.Contains(orders, "shared database") {
		t.Error("orders-svc should keep its own schema only")
	}
	out, _ := ws.LoadContext(LoadOptions{Services: []string{"orders-svc", "payments-svc"}, SkipRules: true, Files: []string{"database.md"}})
	if !strings.Contains(out, "### `orders`") || !strings.Contains(out, "<!-- context/_db/core.md -->") {
		t.Error("loading both should give orders' own schema and the shared core schema")
	}
}

func TestSyncPrismaSource(t *testing.T) {
	ws := dbWS(t)
	put(t, ws.Root, "payments-svc/prisma/schema.prisma", "model Charge {\n  id Int @id @default(autoincrement())\n  amount Int\n  @@map(\"charges\")\n}\n")
	msg, err := ws.SyncDatabase(SyncInput{Service: "payments-svc"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "prisma/schema.prisma") || !strings.Contains(read(t, ws, "payments-svc/database.md"), "### `charges`") {
		t.Errorf("prisma sync: %s", msg)
	}
}
