package main

import (
	"database/sql"
)

type databaseConfig struct {
	Name                   string `json:"name"`
	Database               string `json:"database"`
	Schema                 string `json:"schema"`
	User                   string `json:"user"`
	Password               string `json:"password"`
	Host                   string `json:"host"`
	ConnectOptionsTemplate string `json:"connect_template"`
	ConnectOptions         string `json:"connect_opts"`
}

type web6Config struct {
	DefaultDatabase databaseConfig            `json:"default_database"`
	Databases       map[string]databaseConfig `json:"databases"`
}

type dbFile struct {
	Id           sql.NullInt64
	ThemeId      sql.NullInt64
	TypeId       sql.NullInt64
	Name         sql.NullString
	Path         sql.NullString
	Info         sql.NullString
	Html         sql.NullString
	UdnJson      sql.NullString
	FusePathName string
}

type File struct {
	database *sql.DB
	file     *dbFile
}

type FS struct {
	database *sql.DB
}

// Dir implements both Node and Handle for the root directory.
type Dir struct {
	database *sql.DB
	// nil for the root directory
	file *dbFile
}
