package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TxBeginner is implemented by pgxpool.Pool and can start a DB transaction.
type TxBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// TxFunc is the unit of work executed inside a transaction.
type TxFunc func(ctx context.Context, tx pgx.Tx) error

// WithTx runs fn in a transaction and guarantees rollback on error/panic.
func WithTx(ctx context.Context, db TxBeginner, fn TxFunc) (err error) {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}

		if err != nil {
			_ = tx.Rollback(ctx)
			return
		}

		if commitErr := tx.Commit(ctx); commitErr != nil {
			err = fmt.Errorf("commit transaction: %w", commitErr)
		}
	}()

	err = fn(ctx, tx)
	if err != nil {
		err = fmt.Errorf("transaction body: %w", err)
	}

	return err
}
