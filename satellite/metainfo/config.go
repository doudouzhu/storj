// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package metainfo

import (
	"go.uber.org/zap"

	"storj.io/storj/internal/dbutil"
	"storj.io/storj/internal/memory"
	"storj.io/storj/storage"
	"storj.io/storj/storage/boltdb"
	"storj.io/storj/storage/postgreskv"
)

const (
	// BoltPointerBucket is the string representing the bucket used for `PointerEntries` in BoltDB
	BoltPointerBucket = "pointers"
)

// Config is a configuration struct that is everything you need to start a metainfo
type Config struct {
	DatabaseURL          string      `help:"the database connection string to use" releaseDefault:"postgres://" devDefault:"bolt://$CONFDIR/pointerdb.db"`
	MinRemoteSegmentSize memory.Size `default:"1240" help:"minimum remote segment size"`
	MaxInlineSegmentSize memory.Size `default:"8000" help:"maximum inline segment size"`
	Overlay              bool        `default:"true" help:"toggle flag if overlay is enabled"`
	BwExpiration         int         `default:"45"   help:"lifespan of bandwidth agreements in days"`
}

// NewStore returns database for storing pointer data
func NewStore(logger *zap.Logger, dbURLString string) (db storage.KeyValueStore, err error) {
	driver, source, err := dbutil.SplitConnstr(dbURLString)
	if err != nil {
		return nil, err
	}
	if driver == "bolt" {
		db, err = boltdb.New(source, BoltPointerBucket)
	} else if driver == "postgresql" || driver == "postgres" {
		db, err = postgreskv.New(source)
	} else {
		err = Error.New("unsupported db scheme: %s", driver)
	}
	logger.Debug("Connected to:", zap.String("db source", source))
	return db, err
}
