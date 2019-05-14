// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

// Package pgutil contains utilities for postgres
package pgutil

import (
	"database/sql"
	"encoding/hex"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
)

// CreateRandomTestingSchemaName creates a random schema name string.
func CreateRandomTestingSchemaName(n int) string {
	data := make([]byte, n)

	// math/rand.Read() always returns a nil error so there's no need to handle the error.
	_, _ = rand.Read(data)
	return hex.EncodeToString(data)
}

// ConnstrWithSchema adds schema to a  connection string
func ConnstrWithSchema(connstr, schema string) string {
	schema = strings.ToLower(schema)
	return connstr + "&search_path=" + url.QueryEscape(schema)
}

// ParseSchemaFromConnstr returns the name of the schema parsed from the
// connection string if one is provided
func ParseSchemaFromConnstr(connstr string) (string, error) {
	url, err := url.Parse(connstr)
	if err != nil {
		return "", err
	}
	queryValues := url.Query()
	schema := queryValues["search_path"]
	if len(schema) > 0 {
		return schema[0], nil
	}
	return "", nil
}

// QuoteSchema quotes schema name for
func QuoteSchema(schema string) string {
	return strconv.QuoteToASCII(schema)
}

// Execer is for executing sql
type Execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// CreateSchema creates a schema if it doesn't exist.
func CreateSchema(db Execer, schema string) error {
	_, err := db.Exec(`create schema if not exists ` + QuoteSchema(schema) + `;`)
	return err
}

// DropSchema drops the named schema
func DropSchema(db Execer, schema string) error {
	_, err := db.Exec(`drop schema ` + QuoteSchema(schema) + ` cascade;`)
	return err
}
