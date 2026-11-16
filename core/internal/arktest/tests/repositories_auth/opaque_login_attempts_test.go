//go:build integration

package authrepo_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/internal/arktest"
	authrepo "arkive/core/repositories/auth"
)

func TestConsumeOpaqueLoginAttemptEnforcesBindingExpiryAndSingleUse(t *testing.T) {
	env := arktest.NewEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	user, err := arktest.CreateVerifiedUser(ctx, env.DB, "Test User", "authtest", "authtest@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	userID := user.ID

	repo := authrepo.New()
	attemptNumber := 0
	create := func(expiresAt time.Time) string {
		t.Helper()
		attemptNumber++
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", attemptNumber)
		if err := repo.CreateOpaqueLoginAttempt(ctx, env.DB, id, &userID, []byte("opaque-state"), expiresAt); err != nil {
			t.Fatal(err)
		}
		return id
	}

	id := create(time.Now().Add(time.Minute))
	tx, err := env.DB.BeginTx(ctx, pgx.TxOptions{})
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
	tx, err = env.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.ConsumeOpaqueLoginAttempt(ctx, tx, id)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("replayed attempt returned %v, want pgx.ErrNoRows", err)
	}

	id = create(time.Now().Add(-time.Minute))
	tx, err = env.DB.BeginTx(ctx, pgx.TxOptions{})
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
			tx, err := env.DB.BeginTx(ctx, pgx.TxOptions{})
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
