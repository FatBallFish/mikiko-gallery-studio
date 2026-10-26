package ent

import "entgo.io/ent/dialect"

// HistoryDriver exposes the existing connection for the additive history table.
func (c *Client) HistoryDriver() dialect.Driver { return c.driver }
