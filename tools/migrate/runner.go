package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
)

// Runner defines a migration runner.
type Runner struct {
	db         dbx.Builder
	migrations MigrationsList
	tableName  string
}

// NewRunner creates a new runner instance.
func NewRunner(db dbx.Builder, migrations MigrationsList) *Runner {
	return &Runner{
		db:         db,
		migrations: migrations,
		tableName:  "_migrations",
	}
}

// SetTableName sets the name of the migrations table.
func (r *Runner) SetTableName(name string) {
	r.tableName = name
}

// Run runs the migrations.
//
// Deprecated: Use Up() instead.
func (r *Runner) Run() ([]string, error) {
	return r.Up()
}

// Up runs all pending migrations.
func (r *Runner) Up() ([]string, error) {
	if err := r.createMigrationsTable(); err != nil {
		return nil, err
	}

	applied, err := r.appliedMigrations()
	if err != nil {
		return nil, err
	}

	var appliedNames []string

	for _, migration := range r.migrations {
		if _, ok := applied[migration.Name]; ok {
			continue
		}

		if err := r.run(migration, "up"); err != nil {
			return appliedNames, err
		}

		appliedNames = append(appliedNames, migration.Name)
	}

	return appliedNames, nil
}

// Down reverts the last limit migrations.
func (r *Runner) Down(limit int) ([]string, error) {
	if err := r.createMigrationsTable(); err != nil {
		return nil, err
	}

	applied, err := r.appliedMigrations()
	if err != nil {
		return nil, err
	}

	var revertedNames []string

	// revert in reverse order
	for i := len(r.migrations) - 1; i >= 0; i-- {
		migration := r.migrations[i]

		if _, ok := applied[migration.Name]; !ok {
			continue
		}

		if err := r.run(migration, "down"); err != nil {
			return revertedNames, err
		}

		revertedNames = append(revertedNames, migration.Name)

		if limit > 0 && len(revertedNames) >= limit {
			break
		}
	}

	return revertedNames, nil
}

func (r *Runner) run(migration *Migration, op string) error {
	var tx *dbx.Tx
	var err error

	if db, ok := r.db.(*dbx.DB); ok {
		tx, err = db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// use tx if available, otherwise fallback to r.db
	var executor dbx.Builder = r.db
	if tx != nil {
		executor = tx
	}

	if op == "up" {
		if migration.Up != nil {
			err = migration.Up(executor)
		}
	} else {
		if migration.Down != nil {
			err = migration.Down(executor)
		}
	}

	if err != nil {
		return err
	}

	if op == "up" {
		_, err = executor.Insert(r.tableName, dbx.Params{
			"name":    migration.Name,
			"applied": time.Now().Unix(),
		}).Execute()
	} else {
		_, err = executor.Delete(r.tableName, dbx.HashExp{"name": migration.Name}).Execute()
	}

	if err != nil {
		return err
	}

	if tx != nil {
		return tx.Commit()
	}

	return nil
}

func (r *Runner) createMigrationsTable() error {
	var exists bool

	err := r.db.Select("1").
		From(r.tableName).
		Limit(1).
		Row(&exists)

	if err == nil {
		return nil
	}

	// table doesn't exist, create it
	_, err = r.db.CreateTable(r.tableName, map[string]string{
		"name":    "string PRIMARY KEY",
		"applied": "integer",
	}).Execute()

	return err
}

func (r *Runner) appliedMigrations() (map[string]int64, error) {
	var rows []struct {
		Name    string `db:"name"`
		Applied int64  `db:"applied"`
	}

	err := r.db.Select("name", "applied").
		From(r.tableName).
		All(&rows)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		result[row.Name] = row.Applied
	}

	return result, nil
}
