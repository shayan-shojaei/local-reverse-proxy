package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shayan/local-reverse-proxy/internal/domain"
	_ "modernc.org/sqlite"
)

var (
	ErrHostnameConflict = errors.New("hostname already exists")
	ErrRevisionConflict = errors.New("route revision conflict")
	ErrNotFound         = errors.New("route not found")
)

type Settings struct {
	Zone string `json:"zone"`
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			zone TEXT NOT NULL
		);
		INSERT OR IGNORE INTO settings (id, zone) VALUES (1, 'local.test');
		CREATE TABLE IF NOT EXISTS routes (
			id TEXT PRIMARY KEY,
			hostname TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL,
			public_mode TEXT NOT NULL,
			upstream_scheme TEXT NOT NULL,
			upstream_host TEXT NOT NULL,
			upstream_port INTEGER NOT NULL,
			skip_tls_verify INTEGER NOT NULL,
			revision INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`)
	if err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) CreateRoute(ctx context.Context, route domain.Route) error {
	err := insertRoute(ctx, s.db, route)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return ErrHostnameConflict
	}
	if err != nil {
		return fmt.Errorf("create route: %w", err)
	}
	return nil
}

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertRoute(ctx context.Context, target executor, route domain.Route) error {
	_, err := target.ExecContext(ctx, `INSERT INTO routes
		(id, hostname, enabled, public_mode, upstream_scheme, upstream_host, upstream_port, skip_tls_verify, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		route.ID, route.Hostname, route.Enabled, route.PublicMode, route.Upstream.Scheme, route.Upstream.Host,
		route.Upstream.Port, route.Upstream.SkipTLSVerify, route.Revision, formatTime(route.CreatedAt), formatTime(route.UpdatedAt))
	return err
}

func (s *Store) ReplaceRoutes(ctx context.Context, routes []domain.Route) error {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin route replacement: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, `DELETE FROM routes`); err != nil {
		return fmt.Errorf("clear routes: %w", err)
	}
	for _, route := range routes {
		if err := insertRoute(ctx, transaction, route); err != nil {
			return fmt.Errorf("insert replacement route %q: %w", route.Hostname, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit route replacement: %w", err)
	}
	return nil
}

func (s *Store) ListRoutes(ctx context.Context) ([]domain.Route, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, hostname, enabled, public_mode, upstream_scheme, upstream_host,
		upstream_port, skip_tls_verify, revision, created_at, updated_at FROM routes ORDER BY hostname`)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()
	routes := make([]domain.Route, 0)
	for rows.Next() {
		route, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

func (s *Store) GetRoute(ctx context.Context, id string) (domain.Route, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, hostname, enabled, public_mode, upstream_scheme, upstream_host,
		upstream_port, skip_tls_verify, revision, created_at, updated_at FROM routes WHERE id = ?`, id)
	route, err := scanRoute(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Route{}, ErrNotFound
	}
	return route, err
}

func (s *Store) UpdateRoute(ctx context.Context, route domain.Route, expectedRevision int64) (domain.Route, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE routes SET hostname = ?, enabled = ?, public_mode = ?, upstream_scheme = ?,
		upstream_host = ?, upstream_port = ?, skip_tls_verify = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND revision = ?`, route.Hostname, route.Enabled, route.PublicMode, route.Upstream.Scheme,
		route.Upstream.Host, route.Upstream.Port, route.Upstream.SkipTLSVerify, formatTime(now), route.ID, expectedRevision)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return domain.Route{}, ErrHostnameConflict
	}
	if err != nil {
		return domain.Route{}, fmt.Errorf("update route: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		if _, getErr := s.GetRoute(ctx, route.ID); errors.Is(getErr, ErrNotFound) {
			return domain.Route{}, ErrNotFound
		}
		return domain.Route{}, ErrRevisionConflict
	}
	return s.GetRoute(ctx, route.ID)
}

func (s *Store) DeleteRoute(ctx context.Context, id string, expectedRevision int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM routes WHERE id = ? AND revision = ?`, id, expectedRevision)
	if err != nil {
		return fmt.Errorf("delete route: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		if _, getErr := s.GetRoute(ctx, id); errors.Is(getErr, ErrNotFound) {
			return ErrNotFound
		}
		return ErrRevisionConflict
	}
	return nil
}

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	var settings Settings
	if err := s.db.QueryRowContext(ctx, `SELECT zone FROM settings WHERE id = 1`).Scan(&settings.Zone); err != nil {
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	return settings, nil
}

func (s *Store) SetZone(ctx context.Context, zone string) error {
	if err := domain.ValidateZone(zone); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE settings SET zone = ? WHERE id = 1`, zone)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRoute(row scanner) (domain.Route, error) {
	var route domain.Route
	var createdAt, updatedAt string
	err := row.Scan(&route.ID, &route.Hostname, &route.Enabled, &route.PublicMode, &route.Upstream.Scheme,
		&route.Upstream.Host, &route.Upstream.Port, &route.Upstream.SkipTLSVerify, &route.Revision, &createdAt, &updatedAt)
	if err != nil {
		return domain.Route{}, err
	}
	route.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Route{}, fmt.Errorf("parse created_at: %w", err)
	}
	route.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	route.Health = domain.HealthUnknown
	return route, err
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
