package pointbreak

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDatabaseOrNetworkAccess guards a product promise: Pointbreak never connects to a
// database or the network. It tracks schemas from migration files only. Any import of a
// database driver or network package in non-test code fails this test.
func TestNoDatabaseOrNetworkAccess(t *testing.T) {
	forbidden := []string{"database/sql", "net", "net/", "crypto/tls", "github.com/lib/pq", "github.com/jackc/", "github.com/go-sql-driver/", "gorm.io/", "go.mongodb.org/", "github.com/redis/"}
	fset := token.NewFileSet()
	checked := 0
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "examples" || d.Name() == "bin" || d.Name() == "bench") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		checked++
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				if path == bad || (strings.HasSuffix(bad, "/") && strings.HasPrefix(path, bad)) {
					t.Errorf("%s imports %s: Pointbreak must not open database or network connections", p, path)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 8 {
		t.Fatalf("only %d Go files checked; the walk is broken", checked)
	}
}
