package main

import (
	"database/sql"
	"time"

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
		err = c.createTables()
		if err != nil {
			return
		}
	}

	_, err = c.db.Exec(`UPDATE meta SET lastopen = $1;`, time.Now())

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
    unique_ordinal int PRIMARY KEY DEFAULT 1,
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

	_, err = tx.Exec(`
CREATE TABLE user
(
  id varchar PRIMARY KEY,
  nsid varchar,
  name varchar
);

CREATE TABLE photo
(
  id varchar PRIMARY KEY,
  owner varchar,
  secret varchar,
  server varchar,
  farm varchar,
  title varchar,
  ispublic bool not null,
  isfriend bool not null,
  isfamily bool not null,
  
  lastfetched datetime not null
);

CREATE TABLE photoinfo
(
  id varchar PRIMARY KEY,
  secret varchar,
  views integer,
  faves integer,
  dateposted timestamp,
  datetaken timestamp,
  takengranularity int,
  lastupdate timestamp
);

CREATE TABLE url
(
  id varchar,
  type varchar,
  value varchar,
  PRIMARY KEY (id, type) 
);
`)
	return
}
