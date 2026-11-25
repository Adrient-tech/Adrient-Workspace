package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/demoseed"
	"github.com/jackc/pgx/v5/pgxpool"
)

// bulkseed inserts ADRIENT_BULK_TICKETS desk tickets into the demo org and
// times the desk list query, for the 100k scale check.
func main() {
	dsn := os.Getenv("ADRIENT_POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://adrient@127.0.0.1:5533/adrient?sslmode=disable"
	}
	n := 100000
	if v := os.Getenv("ADRIENT_BULK_TICKETS"); v != "" {
		fmt.Sscanf(v, "%d", &n)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	orgSlug := "default"
	if s := os.Getenv("ADRIENT_ORG_SLUG"); s != "" {
		orgSlug = s
	}
	took, err := demoseed.BulkTickets(ctx, pool, orgSlug, n)
	if err != nil {
		panic(err)
	}
	fmt.Printf("inserted %d tickets in %s\n", n, took)

	var total int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM adrient.tickets`).Scan(&total)
	start := time.Now()
	rows, err := pool.Query(ctx, `SELECT id, number, title, status, priority, created_at
		FROM adrient.tickets ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		panic(err)
	}
	cnt := 0
	for rows.Next() {
		cnt++
	}
	rows.Close()
	fmt.Printf("count=%d list50=%d rows in %v\n", total, cnt, time.Since(start))

	start = time.Now()
	var filtered int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM adrient.tickets WHERE status='open' AND priority='urgent'`).Scan(&filtered); err != nil {
		panic(err)
	}
	fmt.Printf("status-filter count=%d in %v\n", filtered, time.Since(start))
}
