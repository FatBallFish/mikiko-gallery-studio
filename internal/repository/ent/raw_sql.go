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

func (c *Client) DialectName() string { return c.config.driver.Dialect() }
