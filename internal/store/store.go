// Package store is the orchestrator's single SQLite database (ANALIZ.md §3.8 "Store",
// upstream src/app/store.ts): one file in the state directory, one process at a time.
//
// Records are JSON documents; generated columns expose what queries filter on, so a pass
// reads only live work. Nothing deletes old records yet (A9).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	_ "modernc.org/sqlite"
)

// ErrLocked means another process holds the state directory.
var ErrLocked = errors.New("store: state directory is in use by another process")

// Store is an open state directory.
type Store struct {
	db   *sql.DB
	lock *os.File
}

// Open locks dir, opens dir/orchestrator.sqlite and brings its schema up to date.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	dsn := "file:" + filepath.Join(dir, "orchestrator.sqlite") +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		lock.Close()
		return nil, err
	}
	// The engine is the single writer and its methods are synchronous (upstream
	// docs/007); one connection keeps every transaction serialized.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, lock: lock}
	if err := s.migrate(context.Background()); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database and releases the directory lock.
func (s *Store) Close() error {
	err := s.db.Close()
	if s.lock != nil {
		s.lock.Close() // closing the descriptor drops the flock
	}
	return err
}

// Tx runs fn in one transaction, committing when it returns nil.
func (s *Store) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// DB exposes the handle for packages that own their tables (effects, outbox).
func (s *Store) DB() *sql.DB { return s.db }

// SchemaVersion is the newest schema this build knows. It only increases; a database
// written by a newer build is refused rather than misread.
var SchemaVersion = len(migrations)

// migrations[i] takes the schema from version i to i+1. Never edit a released entry;
// append a new one.
var migrations = []string{
	// 1: the domain tables and the queues.
	`
CREATE TABLE trigger (
  id          TEXT PRIMARY KEY,
  body        TEXT NOT NULL,
  disposition TEXT,
  source      TEXT GENERATED ALWAYS AS (json_extract(body, '$.source')) VIRTUAL,
  thread_key  TEXT GENERATED ALWAYS AS (json_extract(body, '$.threads[0].source') || ':' || json_extract(body, '$.threads[0].id')) VIRTUAL,
  at          TEXT GENERATED ALWAYS AS (json_extract(body, '$.at')) VIRTUAL
);
CREATE INDEX trigger_thread ON trigger(thread_key, at);

CREATE TABLE thread (
  key  TEXT PRIMARY KEY,
  body TEXT NOT NULL
);

CREATE TABLE ticket (
  id         TEXT PRIMARY KEY,
  body       TEXT NOT NULL,
  open       INTEGER GENERATED ALWAYS AS (json_type(body, '$.ended') IS NULL) VIRTUAL,
  short_name TEXT GENERATED ALWAYS AS (json_extract(body, '$.execution.name')) VIRTUAL,
  created_at TEXT GENERATED ALWAYS AS (json_extract(body, '$.createdAt')) VIRTUAL
);
CREATE INDEX ticket_open ON ticket(open, created_at);
CREATE UNIQUE INDEX ticket_short_name ON ticket(short_name) WHERE short_name IS NOT NULL;

CREATE TABLE decision (
  id   TEXT PRIMARY KEY,
  body TEXT NOT NULL
);

CREATE TABLE effect (
  id         INTEGER PRIMARY KEY,
  kind       TEXT NOT NULL,
  key        TEXT NOT NULL UNIQUE,
  payload    TEXT NOT NULL,
  state      TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'done', 'dead')),
  attempts   INTEGER NOT NULL DEFAULT 0,
  next_at    INTEGER NOT NULL,
  last_error TEXT,
  created_at INTEGER NOT NULL
);
CREATE INDEX effect_due ON effect(state, kind, next_at);

CREATE TABLE outbox (
  id           TEXT PRIMARY KEY,
  ticket_id    TEXT,
  conversation TEXT NOT NULL,
  seq          INTEGER NOT NULL,
  body         TEXT NOT NULL,
  state        TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'sent')),
  receipt      TEXT,
  created_at   INTEGER NOT NULL,
  sent_at      INTEGER
);
CREATE INDEX outbox_pending ON outbox(state, conversation, seq);

CREATE TABLE kv (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
`,
}

func (s *Store) migrate(ctx context.Context) error {
	var current int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > SchemaVersion {
		return fmt.Errorf("store: database schema %d is newer than this build (%d)", current, SchemaVersion)
	}
	for v := current; v < SchemaVersion; v++ {
		err := s.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1))
			return err
		})
		if err != nil {
			return fmt.Errorf("store: migration %d: %w", v+1, err)
		}
	}
	return nil
}
