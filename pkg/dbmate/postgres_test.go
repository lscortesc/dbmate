package dbmate

import (
	"database/sql"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func postgresTestURL(t *testing.T) *url.URL {
	u, err := url.Parse("postgres://postgres:postgres@postgres/dbmate?sslmode=disable")
	require.Nil(t, err)

	return u
}

func prepTestPostgresDB(t *testing.T) *sql.DB {
	drv := PostgresDriver{}
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// connect database
	db, err := sql.Open("postgres", u.String())
	require.Nil(t, err)

	return db
}

func TestPostgresCreateDropDatabase(t *testing.T) {
	drv := PostgresDriver{}
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// check that database exists and we can connect to it
	func() {
		db, err := sql.Open("postgres", u.String())
		require.Nil(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.Nil(t, err)
	}()

	// drop the database
	err = drv.DropDatabase(u)
	require.Nil(t, err)

	// check that database no longer exists
	func() {
		db, err := sql.Open("postgres", u.String())
		require.Nil(t, err)
		defer mustClose(db)

		err = db.Ping()
		require.NotNil(t, err)
		require.Equal(t, "pq: database \"dbmate\" does not exist", err.Error())
	}()
}

func TestPostgresDumpSchema(t *testing.T) {
	drv := PostgresDriver{}
	u := postgresTestURL(t)

	// prepare database
	db := prepTestPostgresDB(t)
	defer mustClose(db)
	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.Nil(t, err)
	err = drv.InsertMigration(db, "abc2")
	require.Nil(t, err)

	// DumpSchema should return schema
	schema, err := drv.DumpSchema(u, db)
	require.Nil(t, err)
	require.Contains(t, string(schema), "CREATE TABLE schema_migrations")
	require.Contains(t, string(schema), "\n--\n"+
		"-- PostgreSQL database dump complete\n"+
		"--\n\n\n"+
		"--\n"+
		"-- Dbmate schema migrations\n"+
		"--\n\n"+
		"INSERT INTO schema_migrations (version) VALUES\n"+
		"    ('abc1'),\n"+
		"    ('abc2');\n")

	// DumpSchema should return error if command fails
	u.Path = "/fakedb"
	schema, err = drv.DumpSchema(u, db)
	require.Nil(t, schema)
	require.NotNil(t, err)
	require.Equal(t, "pg_dump: [archiver (db)] connection to database \"fakedb\" failed: "+
		"FATAL:  database \"fakedb\" does not exist", err.Error())
}

func TestPostgresDatabaseExists(t *testing.T) {
	drv := PostgresDriver{}
	u := postgresTestURL(t)

	// drop any existing database
	err := drv.DropDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return false
	exists, err := drv.DatabaseExists(u)
	require.Nil(t, err)
	require.Equal(t, false, exists)

	// create database
	err = drv.CreateDatabase(u)
	require.Nil(t, err)

	// DatabaseExists should return true
	exists, err = drv.DatabaseExists(u)
	require.Nil(t, err)
	require.Equal(t, true, exists)
}

func TestPostgresDatabaseExists_Error(t *testing.T) {
	drv := PostgresDriver{}
	u := postgresTestURL(t)
	u.User = url.User("invalid")

	exists, err := drv.DatabaseExists(u)
	require.Equal(t, "pq: role \"invalid\" does not exist", err.Error())
	require.Equal(t, false, exists)
}

func TestPostgresCreateMigrationsTable(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	// migrations table should not exist
	count := 0
	err := db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Equal(t, "pq: relation \"schema_migrations\" does not exist", err.Error())

	// create table
	err = drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	// migrations table should exist
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)

	// create table should be idempotent
	err = drv.CreateMigrationsTable(db)
	require.Nil(t, err)
}

func TestPostgresSelectMigrations(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc2'), ('abc1'), ('abc3')`)
	require.Nil(t, err)

	migrations, err := drv.SelectMigrations(db, -1)
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc1"])
	require.Equal(t, true, migrations["abc2"])
	require.Equal(t, true, migrations["abc2"])

	// test limit param
	migrations, err = drv.SelectMigrations(db, 1)
	require.Nil(t, err)
	require.Equal(t, true, migrations["abc3"])
	require.Equal(t, false, migrations["abc1"])
	require.Equal(t, false, migrations["abc2"])
}

func TestPostgresInsertMigration(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 0, count)

	// insert migration
	err = drv.InsertMigration(db, "abc1")
	require.Nil(t, err)

	err = db.QueryRow("select count(*) from schema_migrations where version = 'abc1'").
		Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}

func TestPostgresDeleteMigration(t *testing.T) {
	drv := PostgresDriver{}
	db := prepTestPostgresDB(t)
	defer mustClose(db)

	err := drv.CreateMigrationsTable(db)
	require.Nil(t, err)

	_, err = db.Exec(`insert into schema_migrations (version)
		values ('abc1'), ('abc2')`)
	require.Nil(t, err)

	err = drv.DeleteMigration(db, "abc2")
	require.Nil(t, err)

	count := 0
	err = db.QueryRow("select count(*) from schema_migrations").Scan(&count)
	require.Nil(t, err)
	require.Equal(t, 1, count)
}
