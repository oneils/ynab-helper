// Package sqlite provides SQLite storage implementations.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Config represents the configuration needed to connect to SQLite.
type Config struct {
	Path       string `long:"sqlite-path" env:"SQLITE_DB_PATH" description:"SQLite database file path" default:"./data/ynab.db"`
	DisableWAL bool   `long:"sqlite-disable-wal" env:"SQLITE_DISABLE_WAL" description:"Disable WAL mode (use default DELETE journal mode)"`
}

// DB wraps a SQLite database connection and provides access to all stores.
type DB struct {
	db *sql.DB

	// Stores
	ynabStore          *YnabStore
	transactionStore   *TransactionStore
	patternStore       *PatternStore
	parserMappingStore *ParserMappingStore
}

// New creates a new SQLite connection and initializes all stores.
func New(cfg Config) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open database (path: %s): %w", cfg.Path, err)
	}

	// Configure connection pool settings - use conservative defaults for non-WAL mode
	db.SetMaxOpenConns(1)    // Single connection for non-WAL
	db.SetMaxIdleConns(1)    // Keep one connection ready
	db.SetConnMaxLifetime(0) // Reuse connections indefinitely

	if !cfg.DisableWAL {
		// WAL mode allows multiple concurrent readers with one writer
		db.SetMaxOpenConns(10) // Allow multiple concurrent connections
		db.SetMaxIdleConns(5)  // Keep some connections ready

		// Enable WAL mode for better concurrency
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("enable WAL mode: %w", err)
		}
		slog.Debug("WAL mode enabled")
	}
	if cfg.DisableWAL {
		slog.Info("WAL mode disabled, using default journal mode")
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Set busy timeout to wait up to 5 seconds for locks
	// This is especially important when running on NFS with WAL disabled
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	slog.Debug("SQLite busy_timeout set to 5000ms")

	// Run migrations
	if err := runMigrations(db, cfg.Path); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	slog.Info("SQLite database initialized", "path", cfg.Path)

	database := &DB{
		db:                 db,
		ynabStore:          NewYnabStore(db),
		transactionStore:   NewTransactionStore(db),
		patternStore:       NewPatternStore(db),
		parserMappingStore: NewParserMappingStore(db),
	}

	return database, nil
}

// Close closes the SQLite connection.
func (db *DB) Close(ctx context.Context) error {
	return db.db.Close()
}

// Ping checks if the database connection is healthy.
func (db *DB) Ping(ctx context.Context) error {
	return db.db.PingContext(ctx)
}

// Database returns the underlying SQL database.
func (db *DB) Database() *sql.DB {
	return db.db
}

// YnabStore returns the YNAB store.
func (db *DB) YnabStore() *YnabStore {
	return db.ynabStore
}

// TransactionStore returns the transaction store.
func (db *DB) TransactionStore() *TransactionStore {
	return db.transactionStore
}

// PatternStore returns the pattern store.
func (db *DB) PatternStore() *PatternStore {
	return db.patternStore
}

// ParserMappingStore returns the parser mapping store.
func (db *DB) ParserMappingStore() *ParserMappingStore {
	return db.parserMappingStore
}

// runMigrations runs database migrations using goose.
func runMigrations(db *sql.DB, dbPath string) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	// Use embedded migrations for Docker compatibility
	goose.SetBaseFS(embedMigrations)

	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, err := goose.GetDBVersion(db)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}

	slog.Info("migrations applied successfully", "version", version)
	return nil
}
