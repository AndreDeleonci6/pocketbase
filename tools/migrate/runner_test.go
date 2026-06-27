package migrate

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/pocketbase/dbx"
	_ "github.com/mattn/go-sqlite3"
)

func TestRunner(t *testing.T) {
	db, err := dbx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrations := MigrationsList{
		{
			Name: "1_test",
			Up: func(db dbx.Builder) error {
				_, err := db.CreateTable("test", map[string]string{
					"id": "integer primary key autoincrement",
				}).Execute()
				return err
			},
			Down: func(db dbx.Builder) error {
				_, err := db.DropTable("test").Execute()
				return err
			},
		},
	}

	runner := NewRunner(db, migrations)

	// test up
	applied, err := runner.Up()
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 1 || applied[0] != "1_test" {
		t.Fatalf("Expected 1 applied migration, got %v", applied)
	}

	// test down
	reverted, err := runner.Down(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(reverted) != 1 || reverted[0] != "1_test" {
		t.Fatalf("Expected 1 reverted migration, got %v", reverted)
	}
}

func TestRunnerRollback(t *testing.T) {
	db, err := dbx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrations := MigrationsList{
		{
			Name: "1_fail",
			Up: func(db dbx.Builder) error {
				_, err := db.CreateTable("test_fail", map[string]string{
					"id": "integer primary key autoincrement",
				}).Execute()
				if err != nil {
					return err
				}
				return errors.New("simulated migration failure")
			},
		},
	}

	runner := NewRunner(db, migrations)

	applied, err := runner.Up()
	if err == nil {
		t.Fatal("Expected migration to fail, but it succeeded")
	}
	if len(applied) != 0 {
		t.Fatalf("Expected 0 applied migrations, got %v", applied)
	}

	// Verify table was not created (rolled back)
	var exists bool
	err = db.Select("1").From("test_fail").Limit(1).Row(&exists)
	if err == nil {
		t.Fatal("Expected table test_fail to not exist, but query succeeded")
	}

	// Verify migration is not in the _migrations table
	var count int
	err = db.Select("count(*)").From("_migrations").Row(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("Expected 0 migrations in _migrations table, got %d", count)
	}
}
