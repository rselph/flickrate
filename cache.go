package main

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

const (
	schemaVersion = 1
)

type db struct {
	db *sql.DB
}

func newDB(fname string) (c *db, err error) {
	c = &db{}
	c.db, err = sql.Open("sqlite3", fname)
	if err != nil {
		c = nil
		return
	}

	err = c.load()
	if err != nil {
		c = nil
		return
	}

	return
}

func (c *db) load() (err error) {
	var metaCount int
	row := c.db.QueryRow(`
SELECT count(*) from sqlite_master where type = "table" and name = "meta";
`)
	err = row.Scan(&metaCount)
	if err != nil {
		return
	}
	if metaCount == 0 {
		// There's no meta table.  Assume blank database and initialize it.
		c.createTables()
	}

	return
}

func (c *db) createTables() (err error) {
	var tx *sql.Tx
	tx, err = c.db.Begin()
	if err != nil {
		return
	}
	defer func() {
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	_, err = tx.Exec(`
CREATE TABLE meta
(
    "unique" int PRIMARY KEY DEFAULT 1,
    lastopen timestamp,
    lastclose timestamp,
    schema_version int
);
`)
	if err != nil {
		return
	}

	_, err = tx.Exec(`
insert into meta (schema_version) VALUES ($1);
`,
		schemaVersion)
	if err != nil {
		return
	}

	return
}
