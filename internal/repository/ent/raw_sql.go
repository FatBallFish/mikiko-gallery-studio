package ent

import (
	"context"

	entsql "entgo.io/ent/dialect/sql"
)

// ExecRaw executes repository-owned DDL or coordination SQL through the same
// driver or transaction as the generated client.
func (c *Client) ExecRaw(ctx context.Context, query string, args ...any) error {
	var result entsql.Result
	return c.config.driver.Exec(ctx, query, args, &result)
}

func (tx *Tx) ExecRaw(ctx context.Context, query string, args ...any) error {
	var result entsql.Result
	return tx.config.driver.Exec(ctx, query, args, &result)
}

// ExecRawAffected executes repository-owned parameterized SQL and reports the
// number of changed rows. It is intended for migration-safe upserts where the
// generated builders do not expose ON CONFLICT.
func (tx *Tx) ExecRawAffected(ctx context.Context, query string, args ...any) (int64, error) {
	var result entsql.Result
	if err := tx.config.driver.Exec(ctx, query, args, &result); err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (c *Client) DialectName() string { return c.config.driver.Dialect() }
