package authrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpaqueLoginAttemptsAreBoundOneTimeAndExpire(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the isolated PostgreSQL login-attempt test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("opaque_attempt_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		_ = admin.Close(context.Background())
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	defer func() {
		if pool != nil {
			pool.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		_ = admin.Close(context.Background())
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.MaxConns = 4
	pool, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE opaque_login_attempts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id UUID,
		server_state BYTEA NOT NULL,
		expires_at TIMESTAMPTZ NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}

	repo := New()
	userID := "11111111-1111-1111-1111-111111111111"
	attemptNumber := 0
	create := func(expiresAt time.Time) string {
		t.Helper()
		attemptNumber++
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", attemptNumber)
		if err := repo.CreateOpaqueLoginAttempt(ctx, pool, id, &userID, []byte("opaque-state"), expiresAt); err != nil {
			t.Fatal(err)
		}
		return id
	}

	id := create(time.Now().Add(time.Minute))
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := repo.ConsumeOpaqueLoginAttempt(ctx, tx, id)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if attempt.UserID == nil || *attempt.UserID != userID || string(attempt.State) != "opaque-state" {
		_ = tx.Rollback(ctx)
		t.Fatalf("attempt was not bound to its user and state: %+v", attempt)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.ConsumeOpaqueLoginAttempt(ctx, tx, id)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("replayed attempt returned %v, want pgx.ErrNoRows", err)
	}

	id = create(time.Now().Add(-time.Minute))
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.ConsumeOpaqueLoginAttempt(ctx, tx, id)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired attempt returned %v, want pgx.ErrNoRows", err)
	}

	id = create(time.Now().Add(time.Minute))
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
			ready.Done()
			if err != nil {
				results <- err
				return
			}
			<-start
			_, err = repo.ConsumeOpaqueLoginAttempt(ctx, tx, id)
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-results, <-results
	successes := 0
	if first == nil {
		successes++
	}
	if second == nil {
		successes++
	}
	if successes != 1 {
		t.Fatalf("concurrent consumers had %d successes (errors: %v, %v), want exactly one", successes, first, second)
	}
	for _, result := range []error{first, second} {
		if result != nil && !errors.Is(result, pgx.ErrNoRows) {
			t.Fatalf("concurrent consume returned unexpected error: %v", result)
		}
	}
}
