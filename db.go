package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	dbFileName = ".stopwatch-tui.db"
	timeFormat = time.RFC3339Nano
)

type Session struct {
	ID                    int64
	StartTime             time.Time
	LastPausedAt          *time.Time
	TotalPausedDurationMs int64
	IsFullscreen          bool
	IsStopped             bool
	IsDeleted             bool
	CreatedAt             time.Time
}

type SplitRow struct {
	ID          int64
	SessionID   int64
	SplitTimeMs int64
	LapNumber   int
	Name        string
	IsDeleted   bool
	CreatedAt   time.Time
}

func (s *Session) ElapsedDuration() time.Duration {
	paused := time.Duration(s.TotalPausedDurationMs) * time.Millisecond
	if s.LastPausedAt != nil {
		return s.LastPausedAt.Sub(s.StartTime) - paused
	}
	return time.Since(s.StartTime) - paused
}

func (s *Session) IsRunning() bool {
	return !s.IsStopped && s.LastPausedAt == nil
}

func dbPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dbFileName), nil
}

func openDB() (*sql.DB, error) {
	path, err := dbPath()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, err
	}
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id                       INTEGER PRIMARY KEY AUTOINCREMENT,
			start_time               TEXT NOT NULL,
			last_paused_at           TEXT,
			total_paused_duration_ms INTEGER NOT NULL DEFAULT 0,
			is_fullscreen            INTEGER NOT NULL DEFAULT 1,
			is_stopped               INTEGER NOT NULL DEFAULT 0,
			is_deleted               INTEGER NOT NULL DEFAULT 0,
			created_at               TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS splits (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id    INTEGER NOT NULL REFERENCES sessions(id),
			split_time_ms INTEGER NOT NULL,
			lap_number    INTEGER NOT NULL,
			name          TEXT NOT NULL DEFAULT '',
			is_deleted    INTEGER NOT NULL DEFAULT 0,
			created_at    TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func tsStr(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func parseTS(s string) (time.Time, error) {
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		return time.Parse("2006-01-02 15:04:05.999999999+00:00", s)
	}
	return t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func createSession(db *sql.DB, fullscreen bool) (*Session, error) {
	now := time.Now()
	res, err := db.Exec(
		`INSERT INTO sessions (start_time, is_fullscreen, is_stopped, is_deleted, created_at)
		 VALUES (?, ?, 0, 0, ?)`,
		tsStr(now), boolToInt(fullscreen), tsStr(now),
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Session{
		ID:           id,
		StartTime:    now,
		IsFullscreen: fullscreen,
		CreatedAt:    now,
	}, nil
}

func getActiveSession(db *sql.DB) (*Session, error) {
	row := db.QueryRow(`
		SELECT id, start_time, last_paused_at, total_paused_duration_ms,
		       is_fullscreen, is_stopped, is_deleted, created_at
		FROM sessions
		WHERE is_stopped = 0 AND is_deleted = 0
		ORDER BY created_at DESC
		LIMIT 1
	`)
	var s Session
	var startTimeStr, createdAtStr string
	var lastPausedAtStr sql.NullString
	var isFullscreen, isStopped, isDeleted int

	err := row.Scan(
		&s.ID, &startTimeStr, &lastPausedAtStr,
		&s.TotalPausedDurationMs, &isFullscreen, &isStopped, &isDeleted, &createdAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.StartTime, err = parseTS(startTimeStr)
	if err != nil {
		return nil, fmt.Errorf("parse start_time %q: %w", startTimeStr, err)
	}
	s.CreatedAt, _ = parseTS(createdAtStr)
	s.IsFullscreen = isFullscreen == 1
	s.IsStopped = isStopped == 1
	s.IsDeleted = isDeleted == 1

	if lastPausedAtStr.Valid {
		t, err := parseTS(lastPausedAtStr.String)
		if err != nil {
			return nil, fmt.Errorf("parse last_paused_at %q: %w", lastPausedAtStr.String, err)
		}
		s.LastPausedAt = &t
	}
	return &s, nil
}

func pauseSession(db *sql.DB, id int64) error {
	_, err := db.Exec(
		`UPDATE sessions SET last_paused_at = ? WHERE id = ?`,
		tsStr(time.Now()), id,
	)
	return err
}

func resumeSession(db *sql.DB, id int64, lastPausedAt time.Time) error {
	pausedMs := time.Since(lastPausedAt).Milliseconds()
	_, err := db.Exec(
		`UPDATE sessions
		 SET last_paused_at = NULL,
		     total_paused_duration_ms = total_paused_duration_ms + ?
		 WHERE id = ?`,
		pausedMs, id,
	)
	return err
}

func softDeleteSession(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE sessions SET is_deleted = 1 WHERE id = ?`, id)
	return err
}

func updateSessionFullscreen(db *sql.DB, id int64, fullscreen bool) error {
	_, err := db.Exec(
		`UPDATE sessions SET is_fullscreen = ? WHERE id = ?`,
		boolToInt(fullscreen), id,
	)
	return err
}

func insertSplit(db *sql.DB, sessionID int64, splitTimeMs int64, lapNumber int, name string, createdAt time.Time) (*SplitRow, error) {
	res, err := db.Exec(
		`INSERT INTO splits (session_id, split_time_ms, lap_number, name, is_deleted, created_at)
		 VALUES (?, ?, ?, ?, 0, ?)`,
		sessionID, splitTimeMs, lapNumber, name, tsStr(createdAt),
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &SplitRow{
		ID:          id,
		SessionID:   sessionID,
		SplitTimeMs: splitTimeMs,
		LapNumber:   lapNumber,
		Name:        name,
		CreatedAt:   createdAt,
	}, nil
}

func getSplits(db *sql.DB, sessionID int64) ([]SplitRow, error) {
	rows, err := db.Query(`
		SELECT id, session_id, split_time_ms, lap_number, name, is_deleted, created_at
		FROM splits
		WHERE session_id = ? AND is_deleted = 0
		ORDER BY lap_number ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var splits []SplitRow
	for rows.Next() {
		var s SplitRow
		var createdAtStr string
		var isDeleted int
		if err := rows.Scan(&s.ID, &s.SessionID, &s.SplitTimeMs, &s.LapNumber, &s.Name, &isDeleted, &createdAtStr); err != nil {
			return nil, err
		}
		s.IsDeleted = isDeleted == 1
		s.CreatedAt, _ = parseTS(createdAtStr)
		splits = append(splits, s)
	}
	return splits, rows.Err()
}

func softDeleteSplit(db *sql.DB, id int64) error {
	_, err := db.Exec(`UPDATE splits SET is_deleted = 1 WHERE id = ?`, id)
	return err
}

func mergeSplitUp(db *sql.DB, targetID int64, splitTimeMs int64, createdAt time.Time) error {
	_, err := db.Exec(
		`UPDATE splits SET split_time_ms = ?, created_at = ? WHERE id = ?`,
		splitTimeMs, tsStr(createdAt), targetID,
	)
	return err
}

func updateSplitName(db *sql.DB, id int64, name string) error {
	_, err := db.Exec(`UPDATE splits SET name = ? WHERE id = ?`, name, id)
	return err
}

func saveAllSplitNames(db *sql.DB, splitIDs []int64, names []string) {
	for i, id := range splitIDs {
		if i < len(names) {
			updateSplitName(db, id, names[i])
		}
	}
}
