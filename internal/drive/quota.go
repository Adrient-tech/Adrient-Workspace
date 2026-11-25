package drive

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// QuotaBucket is bytes and file count for one file category.
type QuotaBucket struct {
	Category string `json:"category"`
	Bytes    int64  `json:"bytes"`
	Files    int64  `json:"files"`
}

// Quota is the storage usage report for the caller and their org.
type Quota struct {
	UsedBytes      int64         `json:"used_bytes"`       // caller's live files
	OrgUsedBytes   int64         `json:"org_used_bytes"`   // all live files in the org
	TrashBytes     int64         `json:"trash_bytes"`      // caller's trashed files
	VersionBytes   int64         `json:"version_bytes"`    // caller's old versions
	LimitBytes     int64         `json:"limit_bytes"`      // 0 = no limit configured
	TrashRetention int           `json:"trash_retention_days"`
	Breakdown      []QuotaBucket `json:"breakdown"`
}

// Category maps a mime type to a quota category.
func Category(mime string) string {
	switch {
	case len(mime) >= 6 && mime[:6] == "image/":
		return "Images"
	case len(mime) >= 6 && mime[:6] == "video/":
		return "Video"
	case len(mime) >= 6 && mime[:6] == "audio/":
		return "Audio"
	case mime == "application/pdf":
		return "PDF"
	case len(mime) >= 5 && mime[:5] == "text/":
		return "Text"
	}
	switch mime {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/msword":
		return "Documents"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "application/vnd.ms-excel":
		return "Spreadsheets"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation", "application/vnd.ms-powerpoint":
		return "Presentations"
	}
	return "Other"
}

// Quota computes usage for a user inside an org.
func (r *Repository) Quota(ctx context.Context, orgID, userID string, limit int64, retentionDays int) (Quota, error) {
	q := Quota{LimitBytes: limit, TrashRetention: retentionDays, Breakdown: []QuotaBucket{}}
	if err := r.pool.QueryRow(ctx, `SELECT
		COALESCE(SUM(size_bytes) FILTER (WHERE trashed_at IS NULL AND owner_id=$2),0),
		COALESCE(SUM(size_bytes) FILTER (WHERE trashed_at IS NULL),0),
		COALESCE(SUM(size_bytes) FILTER (WHERE trashed_at IS NOT NULL AND owner_id=$2),0)
		FROM adrient.drive_files WHERE org_id=$1 AND storage_key IS NOT NULL`, orgID, userID).
		Scan(&q.UsedBytes, &q.OrgUsedBytes, &q.TrashBytes); err != nil {
		return q, err
	}
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(v.size_bytes),0) FROM adrient.drive_file_versions v
		JOIN adrient.drive_files f ON f.id=v.file_id WHERE f.org_id=$1 AND f.owner_id=$2`, orgID, userID).Scan(&q.VersionBytes)
	rows, err := r.pool.Query(ctx, `SELECT mime_type, COALESCE(SUM(size_bytes),0), COUNT(*) FROM adrient.drive_files
		WHERE org_id=$1 AND owner_id=$2 AND trashed_at IS NULL AND storage_key IS NOT NULL GROUP BY mime_type`, orgID, userID)
	if err != nil {
		return q, err
	}
	defer rows.Close()
	idx := map[string]int{}
	for rows.Next() {
		var mime string
		var b, n int64
		if err := rows.Scan(&mime, &b, &n); err != nil {
			return q, err
		}
		c := Category(mime)
		i, ok := idx[c]
		if !ok {
			q.Breakdown = append(q.Breakdown, QuotaBucket{Category: c})
			i = len(q.Breakdown) - 1
			idx[c] = i
		}
		q.Breakdown[i].Bytes += b
		q.Breakdown[i].Files += n
	}
	return q, rows.Err()
}

// QuotaHandler serves GET /api/v1/drive/quota. The limit comes from
// ADRIENT_DRIVE_QUOTA_BYTES (unset or 0 = no limit shown); retention from
// ADRIENT_DRIVE_TRASH_RETENTION_DAYS (default 30).
func (s *Service) QuotaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o, ook := auth.OrgFromContext(r.Context())
		u, uok := auth.UserFromContext(r.Context())
		if !ook || !uok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		limit, _ := strconv.ParseInt(os.Getenv("ADRIENT_DRIVE_QUOTA_BYTES"), 10, 64)
		days, err := strconv.Atoi(os.Getenv("ADRIENT_DRIVE_TRASH_RETENTION_DAYS"))
		if err != nil || days <= 0 {
			days = 30
		}
		q, err := s.repo.Quota(r.Context(), o.ID, u.ID, limit, days)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(q)
	})
}
