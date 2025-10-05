// /internal/sharding/shardmanager.go

package sharding

import (
	"database/sql"
	"fmt"
	"hash/crc32"
	"strconv"
	"time"
	"log"
	"os"
	"errors"
    "path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// ShardManager manages N sqlite DBs as shards (N >= 1).
// When N == 1 this acts like a single-db mode.
type ShardManager struct {
	dbs       []*sql.DB
	numShards int
}

// NewShardManager creates num shards and ensures the users table exists.
func NewShardManager(num int, baseDir string) (*ShardManager, error) {
	if num < 1 {
		return nil, fmt.Errorf("num shards must be >= 1")
	}
	if baseDir == "" {
        baseDir = "temp"
    }

	// Ensure baseDir exists.
	if err := os.MkdirAll(baseDir, 0755); err != nil {
        return nil, fmt.Errorf("failed create db dir: %w", err)
    }
	
	sm := &ShardManager{numShards: num}
	for i := 0; i < num; i++ {
		fname := filepath.Join(baseDir, fmt.Sprintf("shard_%d.db", i))
		db, err := sql.Open("sqlite3", fname)
		if err != nil {
			return nil, err
		}

		// Keep MaxOpenConns as before (controls concurrency per DB).
		db.SetMaxOpenConns(20)
		// Fixed number of idle conns so behavior is consistent across runs.
		db.SetMaxIdleConns(10)
		// Fixed connection lifetime for realism.
		db.SetConnMaxLifetime(5 * time.Minute)

		// Pragmas that help with concurrency & throughput
        if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
            db.Close()
            return nil, fmt.Errorf("failed set journal_mode WAL: %w", err)
        }
        if _, err := db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
            db.Close()
            return nil, fmt.Errorf("failed set busy_timeout: %w", err)
        }
        if _, err := db.Exec("PRAGMA synchronous = NORMAL;"); err != nil {
            db.Close()
            return nil, fmt.Errorf("failed set synchronous: %w", err)
        }
        if _, err := db.Exec("PRAGMA wal_autocheckpoint = 1000;"); err != nil {
            db.Close()
            return nil, fmt.Errorf("failed set wal_autocheckpoint: %w", err)
        }

		// Ensure users table exists.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT, payload TEXT)`); err != nil {
			db.Close()
			return nil, err
		}
		sm.dbs = append(sm.dbs, db)
	}
	return sm, nil
}

// Close closes all DBs.
func (s *ShardManager) Close() error {
    var allErrors []error
    for _, db := range s.dbs {
        if err := db.Close(); err != nil {
            allErrors = append(allErrors, err)
        }
    }
    if len(allErrors) > 0 {
        return errors.Join(allErrors...)
    }
    return nil
}

func (s *ShardManager) getDBAndIndexFor(id int64) (*sql.DB, int) {
	if s.numShards == 1 {
		return s.dbs[0], 0
	}
	h := crc32.ChecksumIEEE([]byte(strconv.FormatInt(id, 10)))
	idx := int(h % uint32(s.numShards))
	return s.dbs[idx], idx
}

// InsertUser inserts a user record into the shard determined by id.
func (s *ShardManager) InsertUser(id int64, username, payload string) error {
	db, idx := s.getDBAndIndexFor(id)
	_, err := db.Exec(`INSERT OR IGNORE INTO users (id, username, payload) VALUES (?, ?, ?)`, id, username, payload)
	if err != nil {
		log.Printf("InsertUser id=%d shard_idx=%d err=%v", id, idx, err)
	}
	return err
}

// GetUser retrieves a user by id from the correct shard.
func (s *ShardManager) GetUser(id int64) (string, string, error) {
	db, idx := s.getDBAndIndexFor(id)
	var uname, payload string
	err := db.QueryRow(`SELECT username, payload FROM users WHERE id = ?`, id).Scan(&uname, &payload)
	if err != nil {
		log.Printf("GetUser id=%d shard_idx=%d err=%v", id, idx, err)
	}
	return uname, payload, err
}
