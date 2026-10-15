// This offline tool is deliberately separate from the console runtime.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type cellChange struct {
	Table  string `json:"table"`
	Column string `json:"column"`
	ID     int64  `json:"id"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func databasePlan(ctx context.Context, tx *sql.Tx, key []byte) ([]cellChange, error) {
	var plan []cellChange
	for _, table := range []string{"proxy_pools", "proxy_discovery_sources"} {
		rows, err := tx.QueryContext(ctx, "SELECT id, credentials FROM "+table+" ORDER BY id")
		if err != nil {
			return nil, fmt.Errorf("%s: cannot read credentials", table)
		}
		for rows.Next() {
			var id int64
			var before string
			if err = rows.Scan(&id, &before); err != nil {
				rows.Close()
				return nil, errors.New("cannot read credential row")
			}
			after, changed, convertErr := convertCredential(key, before)
			if convertErr != nil {
				rows.Close()
				return nil, fmt.Errorf("%s row %d: credential validation failed; check the existing key", table, id)
			}
			if changed {
				plan = append(plan, cellChange{table, "credentials", id, before, after})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, errors.New("credential scan failed")
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT id, mime_type FROM rag_documents ORDER BY id")
	if err != nil {
		return nil, errors.New("cannot read document types")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var before string
		if err := rows.Scan(&id, &before); err != nil {
			return nil, errors.New("document type scan failed")
		}
		if after := convertMIME(before); after != before {
			plan = append(plan, cellChange{"rag_documents", "mime_type", id, before, after})
		}
	}
	return plan, rows.Err()
}

func applyDatabasePlan(ctx context.Context, tx *sql.Tx, plan []cellChange) error {
	for _, c := range plan {
		// Identifiers come only from the internal plan, never from user input.
		result, err := tx.ExecContext(ctx, "UPDATE "+c.Table+" SET "+c.Column+"=$1 WHERE id=$2 AND "+c.Column+"=$3", c.After, c.ID, c.Before)
		if err != nil {
			return fmt.Errorf("%s row %d: update failed", c.Table, c.ID)
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return errors.New("data changed during migration; transaction aborted")
		}
	}
	return nil
}

func writeExclusive(path string, content []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(content)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func runDatabase(ctx context.Context, keyPath, backup string, apply bool) error {
	key, err := os.ReadFile(keyPath)
	if err != nil || len(key) != 32 {
		return errors.New("provide the existing 32-byte proxy-pool.key; no key will be generated")
	}
	dsn := os.Getenv("YUNKUN_MIGRATION_DSN")
	if dsn == "" {
		return errors.New("YUNKUN_MIGRATION_DSN is required")
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("cannot configure database connection")
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: !apply})
	if err != nil {
		return errors.New("cannot connect to database or start transaction")
	}
	defer tx.Rollback()
	plan, err := databasePlan(ctx, tx, key)
	if err != nil {
		return err
	}
	fmt.Printf("database cells requiring migration: %d\n", len(plan))
	if !apply || len(plan) == 0 {
		return nil
	}
	if backup == "" {
		return errors.New("-backup requires a new private file path")
	}
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return errors.New("cannot prepare encrypted-cell backup")
	}
	if err := writeExclusive(backup, b); err != nil {
		return errors.New("cannot create backup; database has not been modified")
	}
	if err := applyDatabasePlan(ctx, tx, plan); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.New("database commit failed; inspect database before retrying")
	}
	fmt.Println("database transaction committed; backup contains ciphertext and type metadata only")
	return nil
}

func run() error {
	mode := flag.String("mode", "", "database or transcripts")
	key := flag.String("key", "", "existing proxy-pool.key (never generated)")
	root := flag.String("root", "", "existing transcript directory")
	backup := flag.String("backup", "", "new private backup file or directory")
	apply := flag.Bool("apply", false, "write changes (default is check only)")
	stopped := flag.Bool("service-stopped", false, "acknowledge all console/worker processes are stopped")
	flag.Parse()
	if *apply && !*stopped {
		return errors.New("-apply requires -service-stopped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch *mode {
	case "database":
		return runDatabase(ctx, *key, *backup, *apply)
	case "transcripts":
		return runTranscripts(*root, *backup, *apply)
	default:
		return errors.New("specify -mode database or -mode transcripts")
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
