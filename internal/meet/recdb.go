package meet

import (
	"context"
	"fmt"
)

// StartRecording records a recording that just started.
func (r *Repository) StartRecording(ctx context.Context, j RecordingJob) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO adrient.meet_recordings (room_id, org_id, started_by, egress_id, object_key, started_at)
		 VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6) ON CONFLICT (egress_id) DO NOTHING`,
		j.RoomID, j.OrgID, j.StartedBy, j.EgressID, j.Key, j.StartedAt)
	if err != nil {
		return fmt.Errorf("meet.StartRecording: %w", err)
	}
	return nil
}

// FinishRecording stores the outcome of a recording.
func (r *Repository) FinishRecording(ctx context.Context, egressID, status, driveFileID string) error {
	var f any
	if driveFileID != "" {
		f = driveFileID
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE adrient.meet_recordings SET status=$2, drive_file_id=$3::uuid, finished_at=now() WHERE egress_id=$1`,
		egressID, status, f)
	return err
}

// PendingRecordings lists recordings still marked as running.
func (r *Repository) PendingRecordings(ctx context.Context) ([]RecordingJob, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT rc.egress_id, rc.org_id::text, rc.room_id::text, COALESCE(m.code,''), m.name, rc.started_by::text,
		        COALESCE(u.display_name, u.email, ''), rc.object_key, rc.started_at
		   FROM adrient.meet_recordings rc
		   JOIN adrient.meet_rooms m ON m.id = rc.room_id
		   LEFT JOIN adrient.users u ON u.id = rc.started_by
		  WHERE rc.status='recording'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordingJob
	for rows.Next() {
		var j RecordingJob
		if err := rows.Scan(&j.EgressID, &j.OrgID, &j.RoomID, &j.Code, &j.RoomName, &j.StartedBy, &j.StarterNm, &j.Key, &j.StartedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
