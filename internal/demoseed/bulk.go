package demoseed

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BulkTickets inserts n extra desk tickets into the demo org's desk project in
// batches of 1,000, for scale tests. Numbering starts above the project's
// highest existing number. It returns the wall time the inserts took.
func BulkTickets(ctx context.Context, pool *pgxpool.Pool, orgSlug string, n int) (time.Duration, error) {
	if n <= 0 {
		return 0, nil
	}
	start := time.Now()
	var orgID, projectID string
	err := pool.QueryRow(ctx,
		`SELECT p.org_id, p.id
		   FROM adrient.ticket_projects p
		   JOIN adrient.orgs o ON o.id = p.org_id
		  WHERE o.slug = $1
		  ORDER BY p.created_at
		  LIMIT 1`, orgSlug).Scan(&orgID, &projectID)
	if err != nil {
		return 0, fmt.Errorf("find desk project: %w", err)
	}
	var maxNum int64
	if err := pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(number),0) FROM adrient.tickets WHERE project_id = $1`,
		projectID).Scan(&maxNum); err != nil {
		return 0, fmt.Errorf("max number: %w", err)
	}
	done := 0
	for done < n {
		batch := n - done
		if batch > 1000 {
			batch = 1000
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return 0, err
		}
		if err := bulkInsert(ctx, tx, orgID, projectID, int(maxNum)+done+1, batch, time.Now()); err != nil {
			_ = tx.Rollback(ctx)
			return 0, fmt.Errorf("insert batch: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		done += batch
	}
	return time.Since(start), nil
}

// bulkInsert writes one batch of tickets with COPY, which avoids a transaction
// per row.
func bulkInsert(ctx context.Context, tx pgx.Tx, orgID, projectID string, from, count int, today time.Time) error {
	rows := make([][]any, 0, count)
	for i := from; i < from+count; i++ {
		rows = append(rows, []any{
			projectID, orgID, int64(i),
			fmt.Sprintf("Scale test ticket %d", i),
			"Automated scale-test ticket. Safe to delete.",
			statusFor(i), prioFor(i),
			nil, "Scale Tester", "scale@example.com", "web",
		})
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"adrient", "tickets"},
		[]string{"project_id", "org_id", "number", "title", "body", "status", "priority", "requester_user_id", "requester_name", "requester_email", "source"},
		pgx.CopyFromRows(rows))
	return err
}

func statusFor(i int) string {
	switch i % 4 {
	case 0:
		return "open"
	case 1:
		return "pending"
	case 2:
		return "solved"
	default:
		return "closed"
	}
}

func prioFor(i int) string {
	switch i % 5 {
	case 0:
		return "low"
	case 3:
		return "high"
	case 4:
		return "urgent"
	default:
		return "normal"
	}
}
