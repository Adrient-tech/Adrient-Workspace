package video

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
)

// Extras serves the Video app's JSON API under /api/v1/videos/x/: the feed with
// filters, watch page data, visibility, likes, comments, subscriptions, watch
// later, history, owner stats, Meet recordings and Drive import. The gRPC
// service keeps the basic CRUD; nothing here changes its contract.
type Extras struct {
	pool  *pgxpool.Pool
	blobs BlobStore
}

// NewExtras constructs the handler.
func NewExtras(pool *pgxpool.Pool, blobs BlobStore) *Extras {
	return &Extras{pool: pool, blobs: blobs}
}

const xMount = "/api/v1/videos/x/"

// Match reports whether the path belongs to the extras API.
func (e *Extras) Match(p string) bool { return strings.HasPrefix(p, xMount) }

// VisibleClause is the SQL fragment (alias v, $1 org, $2 user) that limits rows
// to videos the user may see: their own, org-wide ones, ones shared with a
// group they belong to, and ones shared with them directly.
const visibleClause = `v.org_id=$1::uuid AND v.trashed_at IS NULL AND (
	v.owner_id=$2::uuid OR v.visibility='org'
	OR (v.visibility='group' AND v.group_id IS NOT NULL AND EXISTS(
		SELECT 1 FROM adrient.groups g WHERE g.id=v.group_id AND (g.owner_id=$2::uuid OR g.member_ids ? $2::text)))
	OR EXISTS(SELECT 1 FROM adrient.video_shares s WHERE s.video_id=v.id AND s.shared_with_user_id=$2::uuid))`

// CanView reports whether the user may watch the video (any org).
func (r *Repository) CanView(ctx context.Context, orgID, userID, videoID string) bool {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.videos v WHERE v.id=$3::uuid AND `+visibleClause+`)`, orgID, userID, videoID).Scan(&ok)
	return err == nil && ok
}

// Info is a video as the web app sees it.
type Info struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	OwnerID       string    `json:"owner_id"`
	OwnerName     string    `json:"owner_name"`
	Mine          bool      `json:"mine"`
	Visibility    string    `json:"visibility"`
	GroupID       string    `json:"group_id"`
	Status        string    `json:"status"`
	Duration      float64   `json:"duration_seconds"`
	Size          int64     `json:"size"`
	ContentType   string    `json:"content_type"`
	ThumbURL      string    `json:"thumb_url"`
	StreamURL     string    `json:"stream_url"`
	SourceKind    string    `json:"source_kind"`
	CreatedAt     time.Time `json:"created_at"`
	Views         int       `json:"views"`
	Likes         int       `json:"likes"`
	Comments      int       `json:"comments"`
	Liked         bool      `json:"liked"`
	ProgressSecs  float64   `json:"progress_seconds"`
	ProgressPct   float64   `json:"progress_percent"`
	Watched       bool      `json:"watched"`
	WatchLater    bool      `json:"watch_later"`
	Subscribed    bool      `json:"subscribed"`
	HasCaptions   bool      `json:"has_captions"`
	ProgressStamp time.Time `json:"-"`
}

const infoSelect = `SELECT v.id::text, v.title, v.description, v.owner_id::text, COALESCE(NULLIF(u.display_name,''), u.email),
	v.visibility, COALESCE(v.group_id::text,''), v.status, v.duration_seconds, v.size, v.content_type,
	(v.thumbnail_data_url <> ''), v.source_kind, v.created_at, v.updated_at,
	(SELECT count(*) FROM adrient.video_views x WHERE x.video_id=v.id),
	(SELECT count(*) FROM adrient.video_likes x WHERE x.video_id=v.id),
	(SELECT count(*) FROM adrient.video_comments x WHERE x.video_id=v.id),
	EXISTS(SELECT 1 FROM adrient.video_likes x WHERE x.video_id=v.id AND x.user_id=$2::uuid),
	COALESCE(pr.position_seconds,0), COALESCE(pr.percent,0), COALESCE(pr.watched,false),
	EXISTS(SELECT 1 FROM adrient.video_watch_later x WHERE x.video_id=v.id AND x.user_id=$2::uuid),
	EXISTS(SELECT 1 FROM adrient.video_subscriptions x WHERE x.channel_user_id=v.owner_id AND x.user_id=$2::uuid),
	EXISTS(SELECT 1 FROM adrient.video_captions x WHERE x.video_id=v.id),
	COALESCE(pr.updated_at, v.created_at)
	FROM adrient.videos v
	JOIN adrient.users u ON u.id=v.owner_id
	LEFT JOIN adrient.video_progress pr ON pr.video_id=v.id AND pr.user_id=$2::uuid`

func scanInfos(rows pgx.Rows, userID string) ([]Info, error) {
	defer rows.Close()
	out := []Info{}
	for rows.Next() {
		var v Info
		var hasThumb bool
		var upd time.Time
		if err := rows.Scan(&v.ID, &v.Title, &v.Description, &v.OwnerID, &v.OwnerName, &v.Visibility, &v.GroupID, &v.Status,
			&v.Duration, &v.Size, &v.ContentType, &hasThumb, &v.SourceKind, &v.CreatedAt, &upd, &v.Views, &v.Likes, &v.Comments,
			&v.Liked, &v.ProgressSecs, &v.ProgressPct, &v.Watched, &v.WatchLater, &v.Subscribed, &v.HasCaptions, &v.ProgressStamp); err != nil {
			return nil, err
		}
		v.Mine = v.OwnerID == userID
		v.StreamURL = "/api/v1/videos/" + v.ID + "/content"
		if hasThumb {
			v.ThumbURL = xMount + "poster/" + v.ID + "?v=" + strconv.FormatInt(upd.Unix(), 10)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// FeedFilter selects videos for the home grid, search and channel pages.
type FeedFilter struct {
	Q        string
	Duration string // short (<4 min), medium, long (>20 min)
	Date     string // today, week, month, year
	Sort     string // new, oldest, views, title
	Owner    string
	Subs     bool // only channels the user subscribes to
	Source   string
	Limit    int
}

// Feed returns the videos the user may see, filtered.
func (e *Extras) Feed(ctx context.Context, orgID, userID string, f FeedFilter) ([]Info, error) {
	where := []string{visibleClause}
	args := []any{orgID, userID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		add("(v.title ILIKE '%' || ? || '%' OR v.description ILIKE '%' || ? || '%' OR u.display_name ILIKE '%' || ? || '%')", q)
	}
	switch f.Duration {
	case "short":
		where = append(where, "v.duration_seconds > 0 AND v.duration_seconds < 240")
	case "medium":
		where = append(where, "v.duration_seconds >= 240 AND v.duration_seconds <= 1200")
	case "long":
		where = append(where, "v.duration_seconds > 1200")
	}
	switch f.Date {
	case "today":
		where = append(where, "v.created_at > now() - interval '1 day'")
	case "week":
		where = append(where, "v.created_at > now() - interval '7 days'")
	case "month":
		where = append(where, "v.created_at > now() - interval '1 month'")
	case "year":
		where = append(where, "v.created_at > now() - interval '1 year'")
	}
	if f.Owner != "" {
		add("v.owner_id = ?::uuid", f.Owner)
	}
	if f.Subs {
		where = append(where, "EXISTS(SELECT 1 FROM adrient.video_subscriptions sb WHERE sb.channel_user_id=v.owner_id AND sb.user_id=$2::uuid)")
	}
	if f.Source != "" {
		add("v.source_kind = ?", f.Source)
	}
	order := "v.created_at DESC"
	switch f.Sort {
	case "oldest":
		order = "v.created_at ASC"
	case "views":
		order = "(SELECT count(*) FROM adrient.video_views x WHERE x.video_id=v.id) DESC, v.created_at DESC"
	case "title":
		order = "lower(v.title) ASC"
	}
	lim := f.Limit
	if lim <= 0 || lim > 300 {
		lim = 120
	}
	rows, err := e.pool.Query(ctx, infoSelect+" WHERE "+strings.Join(where, " AND ")+" ORDER BY "+order+fmt.Sprintf(" LIMIT %d", lim), args...)
	if err != nil {
		return nil, err
	}
	return scanInfos(rows, userID)
}

// One returns a single visible video.
func (e *Extras) One(ctx context.Context, orgID, userID, id string) (Info, error) {
	rows, err := e.pool.Query(ctx, infoSelect+` WHERE v.id=$3::uuid AND `+visibleClause, orgID, userID, id)
	if err != nil {
		return Info{}, err
	}
	l, err := scanInfos(rows, userID)
	if err != nil {
		return Info{}, err
	}
	if len(l) == 0 {
		return Info{}, ErrNotFound
	}
	return l[0], nil
}

// Related returns videos to show beside the player: same channel first, then
// the newest others.
func (e *Extras) Related(ctx context.Context, orgID, userID, id string, limit int) ([]Info, error) {
	if limit <= 0 || limit > 50 {
		limit = 14
	}
	rows, err := e.pool.Query(ctx, infoSelect+` WHERE v.id <> $3::uuid AND `+visibleClause+`
		ORDER BY (v.owner_id = (SELECT owner_id FROM adrient.videos WHERE id=$3::uuid)) DESC, v.created_at DESC LIMIT `+strconv.Itoa(limit), orgID, userID, id)
	if err != nil {
		return nil, err
	}
	return scanInfos(rows, userID)
}

// Update changes the metadata of a video the user owns.
func (e *Extras) Update(ctx context.Context, orgID, userID, id string, in UpdateInput) (Info, error) {
	if in.Visibility != "" {
		switch in.Visibility {
		case "private", "org":
			in.GroupID = ""
		case "group":
			if in.GroupID == "" {
				return Info{}, errors.New("group_id required")
			}
			var ok bool
			if err := e.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.groups g WHERE g.id=$1::uuid AND g.org_id=$2::uuid
				AND (g.owner_id=$3::uuid OR g.member_ids ? $3::text))`, in.GroupID, orgID, userID).Scan(&ok); err != nil || !ok {
				return Info{}, errors.New("not a member of that group")
			}
		default:
			return Info{}, errors.New("visibility must be private, org or group")
		}
	}
	if in.ThumbnailDataURL != nil && len(*in.ThumbnailDataURL) > 600_000 {
		return Info{}, errors.New("thumbnail too large")
	}
	var gid any
	if in.GroupID != "" {
		gid = in.GroupID
	}
	tag, err := e.pool.Exec(ctx, `UPDATE adrient.videos SET
		title = CASE WHEN $4::text IS NULL THEN title ELSE $4 END,
		description = CASE WHEN $5::text IS NULL THEN description ELSE $5 END,
		thumbnail_data_url = CASE WHEN $6::text IS NULL THEN thumbnail_data_url ELSE $6 END,
		visibility = CASE WHEN $7::text = '' THEN visibility ELSE $7 END,
		group_id = CASE WHEN $7::text = '' THEN group_id ELSE $8::uuid END,
		updated_at = now()
		WHERE id=$1::uuid AND org_id=$2::uuid AND owner_id=$3::uuid AND trashed_at IS NULL`,
		id, orgID, userID, in.Title, in.Description, in.ThumbnailDataURL, in.Visibility, gid)
	if err != nil {
		return Info{}, err
	}
	if tag.RowsAffected() == 0 {
		return Info{}, ErrNotFound
	}
	return e.One(ctx, orgID, userID, id)
}

// UpdateInput holds optional metadata changes (nil = leave as is).
type UpdateInput struct {
	Title            *string `json:"title"`
	Description      *string `json:"description"`
	ThumbnailDataURL *string `json:"thumbnail_data_url"`
	Visibility       string  `json:"visibility"`
	GroupID          string  `json:"group_id"`
}

// RecordView stores a view with the seconds watched. The first call for a
// (user, video) pair within 30 minutes creates the row; later calls add time.
func (e *Extras) RecordView(ctx context.Context, orgID, userID, id string, seconds float64) error {
	if !e.CanViewVideo(ctx, orgID, userID, id) {
		return ErrNotFound
	}
	if seconds < 0 || seconds > 86400 {
		seconds = 0
	}
	tag, err := e.pool.Exec(ctx, `UPDATE adrient.video_views SET watched_seconds = watched_seconds + $3
		WHERE id = (SELECT id FROM adrient.video_views WHERE video_id=$1::uuid AND user_id=$2::uuid
			AND created_at > now() - interval '30 minutes' ORDER BY created_at DESC LIMIT 1)`, id, userID, seconds)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		_, err = e.pool.Exec(ctx, `INSERT INTO adrient.video_views (video_id, user_id, watched_seconds) VALUES ($1::uuid,$2::uuid,$3)`, id, userID, seconds)
	}
	return err
}

// CanViewVideo is CanView through the extras pool.
func (e *Extras) CanViewVideo(ctx context.Context, orgID, userID, id string) bool {
	return (&Repository{pool: e.pool}).CanView(ctx, orgID, userID, id)
}

// Stats are the owner-only numbers of a video.
type Stats struct {
	Views         int        `json:"views"`
	UniqueViewers int        `json:"unique_viewers"`
	WatchSeconds  float64    `json:"watch_seconds"`
	AvgSeconds    float64    `json:"avg_seconds"`
	Likes         int        `json:"likes"`
	Comments      int        `json:"comments"`
	Daily         []DayCount `json:"daily"`
}

// DayCount is views on one day.
type DayCount struct {
	Day   string `json:"day"`
	Views int    `json:"views"`
}

// Stats returns view counts and watch time of a video the user owns.
func (e *Extras) Stats(ctx context.Context, orgID, userID, id string) (Stats, error) {
	var s Stats
	err := e.pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT x.user_id), COALESCE(sum(x.watched_seconds),0)
		FROM adrient.video_views x JOIN adrient.videos v ON v.id=x.video_id
		WHERE v.id=$1::uuid AND v.org_id=$2::uuid AND v.owner_id=$3::uuid`, id, orgID, userID).Scan(&s.Views, &s.UniqueViewers, &s.WatchSeconds)
	if err != nil {
		return s, err
	}
	var owner bool
	_ = e.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.videos WHERE id=$1::uuid AND org_id=$2::uuid AND owner_id=$3::uuid)`, id, orgID, userID).Scan(&owner)
	if !owner {
		return Stats{}, ErrNotFound
	}
	if s.Views > 0 {
		s.AvgSeconds = s.WatchSeconds / float64(s.Views)
	}
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.video_likes WHERE video_id=$1::uuid`, id).Scan(&s.Likes)
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.video_comments WHERE video_id=$1::uuid`, id).Scan(&s.Comments)
	rows, err := e.pool.Query(ctx, `SELECT to_char(d::date,'YYYY-MM-DD'), count(x.id) FROM generate_series(current_date - 13, current_date, '1 day') d
		LEFT JOIN adrient.video_views x ON x.video_id=$1::uuid AND x.created_at::date = d::date GROUP BY d ORDER BY d`, id)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	s.Daily = []DayCount{}
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Views); err != nil {
			return s, err
		}
		s.Daily = append(s.Daily, d)
	}
	return s, rows.Err()
}

// SetLike sets or clears the user's like of a video.
func (e *Extras) SetLike(ctx context.Context, orgID, userID, id string, on bool) error {
	if !e.CanViewVideo(ctx, orgID, userID, id) {
		return ErrNotFound
	}
	var err error
	if on {
		_, err = e.pool.Exec(ctx, `INSERT INTO adrient.video_likes (video_id, user_id) VALUES ($1::uuid,$2::uuid) ON CONFLICT DO NOTHING`, id, userID)
	} else {
		_, err = e.pool.Exec(ctx, `DELETE FROM adrient.video_likes WHERE video_id=$1::uuid AND user_id=$2::uuid`, id, userID)
	}
	return err
}

// Comment is one comment or reply.
type Comment struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Likes     int       `json:"likes"`
	Liked     bool      `json:"liked"`
	Mine      bool      `json:"mine"`
	CanDelete bool      `json:"can_delete"`
}

// Comments lists a video's comments oldest first (replies carry parent_id).
func (e *Extras) Comments(ctx context.Context, orgID, userID, id string) ([]Comment, error) {
	if !e.CanViewVideo(ctx, orgID, userID, id) {
		return nil, ErrNotFound
	}
	rows, err := e.pool.Query(ctx, `SELECT c.id::text, COALESCE(c.parent_id::text,''), c.user_id::text, COALESCE(NULLIF(u.display_name,''),u.email),
		c.body, c.created_at,
		(SELECT count(*) FROM adrient.video_comment_likes l WHERE l.comment_id=c.id),
		EXISTS(SELECT 1 FROM adrient.video_comment_likes l WHERE l.comment_id=c.id AND l.user_id=$2::uuid),
		(c.user_id=$2::uuid), (c.user_id=$2::uuid OR v.owner_id=$2::uuid)
		FROM adrient.video_comments c JOIN adrient.users u ON u.id=c.user_id JOIN adrient.videos v ON v.id=c.video_id
		WHERE c.video_id=$1::uuid AND v.org_id=$3::uuid ORDER BY c.created_at`, id, userID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.ParentID, &c.UserID, &c.UserName, &c.Body, &c.CreatedAt, &c.Likes, &c.Liked, &c.Mine, &c.CanDelete); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddComment posts a comment (or a reply when parentID is set; replies attach
// to the top-level comment so threads stay one level deep).
func (e *Extras) AddComment(ctx context.Context, orgID, userID, id, parentID, body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 4000 {
		return "", errors.New("body required (max 4000 characters)")
	}
	if !e.CanViewVideo(ctx, orgID, userID, id) {
		return "", ErrNotFound
	}
	var parent any
	if parentID != "" {
		var top *string
		err := e.pool.QueryRow(ctx, `SELECT COALESCE(parent_id::text, id::text) FROM adrient.video_comments WHERE id=$1::uuid AND video_id=$2::uuid`, parentID, id).Scan(&top)
		if err != nil || top == nil {
			return "", ErrNotFound
		}
		parent = *top
	}
	var cid string
	err := e.pool.QueryRow(ctx, `INSERT INTO adrient.video_comments (org_id, video_id, user_id, parent_id, body)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5) RETURNING id::text`, orgID, id, userID, parent, body).Scan(&cid)
	return cid, err
}

// ToggleCommentLike flips the user's like of a comment and returns the new state.
func (e *Extras) ToggleCommentLike(ctx context.Context, orgID, userID, commentID string) (bool, int, error) {
	var vid string
	if err := e.pool.QueryRow(ctx, `SELECT video_id::text FROM adrient.video_comments WHERE id=$1::uuid AND org_id=$2::uuid`, commentID, orgID).Scan(&vid); err != nil {
		return false, 0, ErrNotFound
	}
	if !e.CanViewVideo(ctx, orgID, userID, vid) {
		return false, 0, ErrNotFound
	}
	tag, err := e.pool.Exec(ctx, `DELETE FROM adrient.video_comment_likes WHERE comment_id=$1::uuid AND user_id=$2::uuid`, commentID, userID)
	if err != nil {
		return false, 0, err
	}
	on := tag.RowsAffected() == 0
	if on {
		if _, err := e.pool.Exec(ctx, `INSERT INTO adrient.video_comment_likes (comment_id, user_id) VALUES ($1::uuid,$2::uuid)`, commentID, userID); err != nil {
			return false, 0, err
		}
	}
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.video_comment_likes WHERE comment_id=$1::uuid`, commentID).Scan(&n)
	return on, n, nil
}

// DeleteComment removes a comment (and its replies) by its author or the video owner.
func (e *Extras) DeleteComment(ctx context.Context, orgID, userID, commentID string) error {
	tag, err := e.pool.Exec(ctx, `DELETE FROM adrient.video_comments c USING adrient.videos v
		WHERE c.id=$1::uuid AND v.id=c.video_id AND c.org_id=$2::uuid AND (c.user_id=$3::uuid OR v.owner_id=$3::uuid)`, commentID, orgID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Channel is a user's video channel.
type Channel struct {
	UserID      string `json:"user_id"`
	Name        string `json:"name"`
	Subscribers int    `json:"subscribers"`
	Videos      int    `json:"videos"`
	Subscribed  bool   `json:"subscribed"`
	Mine        bool   `json:"mine"`
}

// Channel returns the channel of a user (name, counts, whether the caller subscribes).
func (e *Extras) Channel(ctx context.Context, orgID, userID, channelID string) (Channel, error) {
	var c Channel
	err := e.pool.QueryRow(ctx, `SELECT u.id::text, COALESCE(NULLIF(u.display_name,''),u.email),
		(SELECT count(*) FROM adrient.video_subscriptions s WHERE s.channel_user_id=u.id),
		(SELECT count(*) FROM adrient.videos v WHERE v.owner_id=u.id AND `+visibleClause+`),
		EXISTS(SELECT 1 FROM adrient.video_subscriptions s WHERE s.channel_user_id=u.id AND s.user_id=$2::uuid), (u.id=$2::uuid)
		FROM adrient.users u WHERE u.id=$3::uuid AND u.org_id=$1::uuid`, orgID, userID, channelID).
		Scan(&c.UserID, &c.Name, &c.Subscribers, &c.Videos, &c.Subscribed, &c.Mine)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Subscriptions lists the channels the user subscribes to.
func (e *Extras) Subscriptions(ctx context.Context, orgID, userID string) ([]Channel, error) {
	rows, err := e.pool.Query(ctx, `SELECT u.id::text FROM adrient.video_subscriptions s JOIN adrient.users u ON u.id=s.channel_user_id
		WHERE s.user_id=$1::uuid AND u.org_id=$2::uuid ORDER BY COALESCE(NULLIF(u.display_name,''),u.email)`, userID, orgID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []Channel{}
	for _, id := range ids {
		c, err := e.Channel(ctx, orgID, userID, id)
		if err == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

// Subscribe follows or unfollows a channel.
func (e *Extras) Subscribe(ctx context.Context, orgID, userID, channelID string, on bool) error {
	if channelID == userID {
		return errors.New("you cannot subscribe to your own channel")
	}
	if _, err := e.Channel(ctx, orgID, userID, channelID); err != nil {
		return err
	}
	var err error
	if on {
		_, err = e.pool.Exec(ctx, `INSERT INTO adrient.video_subscriptions (user_id, channel_user_id) VALUES ($1::uuid,$2::uuid) ON CONFLICT DO NOTHING`, userID, channelID)
	} else {
		_, err = e.pool.Exec(ctx, `DELETE FROM adrient.video_subscriptions WHERE user_id=$1::uuid AND channel_user_id=$2::uuid`, userID, channelID)
	}
	return err
}

// WatchLater returns the user's watch-later list, newest addition first.
func (e *Extras) WatchLater(ctx context.Context, orgID, userID string) ([]Info, error) {
	rows, err := e.pool.Query(ctx, infoSelect+` JOIN adrient.video_watch_later wl ON wl.video_id=v.id AND wl.user_id=$2::uuid
		WHERE `+visibleClause+` ORDER BY wl.added_at DESC`, orgID, userID)
	if err != nil {
		return nil, err
	}
	return scanInfos(rows, userID)
}

// SetWatchLater adds or removes a video from the user's watch-later list.
func (e *Extras) SetWatchLater(ctx context.Context, orgID, userID, id string, on bool) error {
	if !e.CanViewVideo(ctx, orgID, userID, id) {
		return ErrNotFound
	}
	var err error
	if on {
		_, err = e.pool.Exec(ctx, `INSERT INTO adrient.video_watch_later (user_id, video_id) VALUES ($1::uuid,$2::uuid) ON CONFLICT DO NOTHING`, userID, id)
	} else {
		_, err = e.pool.Exec(ctx, `DELETE FROM adrient.video_watch_later WHERE user_id=$1::uuid AND video_id=$2::uuid`, userID, id)
	}
	return err
}

// History returns videos the user has started, most recently watched first.
func (e *Extras) History(ctx context.Context, orgID, userID string) ([]Info, error) {
	rows, err := e.pool.Query(ctx, infoSelect+` WHERE pr.user_id IS NOT NULL AND `+visibleClause+` ORDER BY pr.updated_at DESC LIMIT 200`, orgID, userID)
	if err != nil {
		return nil, err
	}
	return scanInfos(rows, userID)
}

// ClearHistory forgets the user's watch positions.
func (e *Extras) ClearHistory(ctx context.Context, userID string) error {
	_, err := e.pool.Exec(ctx, `DELETE FROM adrient.video_progress WHERE user_id=$1::uuid`, userID)
	return err
}

// PosterBytes decodes the poster of a visible video.
func (e *Extras) PosterBytes(ctx context.Context, orgID, userID, id string) ([]byte, string, time.Time, error) {
	var du string
	var upd time.Time
	err := e.pool.QueryRow(ctx, `SELECT v.thumbnail_data_url, v.updated_at FROM adrient.videos v WHERE v.id=$3::uuid AND `+visibleClause, orgID, userID, id).Scan(&du, &upd)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && du == "") {
		return nil, "", time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return decodeDataURL(du, upd)
}

// --- Drive and Meet --------------------------------------------------------

var videoExt = map[string]string{".mp4": "video/mp4", ".m4v": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime", ".mkv": "video/x-matroska", ".ogv": "video/ogg"}

func isVideoFile(name, mime string) bool {
	if strings.HasPrefix(mime, "video/") {
		return true
	}
	_, ok := videoExt[strings.ToLower(path.Ext(name))]
	return ok
}

// DriveVideo is one video file of the Drive offered for import.
type DriveVideo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Imported bool   `json:"imported"`
}

// DriveVideoFiles lists the user's video files in Drive.
func (e *Extras) DriveVideoFiles(ctx context.Context, orgID, userID, q string) ([]DriveVideo, error) {
	rows, err := e.pool.Query(ctx, `SELECT f.id::text, f.name, f.mime_type, f.size_bytes,
		EXISTS(SELECT 1 FROM adrient.videos v WHERE v.source_file_id=f.id AND v.trashed_at IS NULL)
		FROM adrient.drive_files f
		WHERE f.org_id=$1::uuid AND f.owner_id=$2::uuid AND f.trashed_at IS NULL AND f.storage_key IS NOT NULL AND f.name ILIKE '%' || $3 || '%'
		ORDER BY f.updated_at DESC LIMIT 400`, orgID, userID, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DriveVideo{}
	for rows.Next() {
		var d DriveVideo
		var mime string
		if err := rows.Scan(&d.ID, &d.Name, &mime, &d.Size, &d.Imported); err != nil {
			return nil, err
		}
		if isVideoFile(d.Name, mime) {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// ImportOpts tunes a Drive import.
type ImportOpts struct {
	Title      string
	Visibility string // default private
	Kind       string // "drive" or "meet"
}

// ImportFromDrive adds a Drive video to the Video app without copying the
// bytes (the video points at the Drive blob). A same-name ".vtt" next to the
// file becomes its caption track. Idempotent per Drive file.
func (e *Extras) ImportFromDrive(ctx context.Context, orgID, userID, fileID string, o ImportOpts) (Info, error) {
	var existing string
	if e.pool.QueryRow(ctx, `SELECT id::text FROM adrient.videos WHERE source_file_id=$1::uuid AND org_id=$2::uuid AND trashed_at IS NULL`, fileID, orgID).Scan(&existing) == nil {
		return e.One(ctx, orgID, userID, existing)
	}
	var name, mime string
	var key *string
	var size int64
	var parent *string
	err := e.pool.QueryRow(ctx, `SELECT name, mime_type, storage_key, size_bytes, parent_id::text FROM adrient.drive_files
		WHERE id=$1::uuid AND org_id=$2::uuid AND trashed_at IS NULL`, fileID, orgID).Scan(&name, &mime, &key, &size, &parent)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && key == nil) {
		return Info{}, ErrNotFound
	}
	if err != nil {
		return Info{}, err
	}
	if !isVideoFile(name, mime) {
		return Info{}, errors.New("not a video file")
	}
	ct := mime
	if !strings.HasPrefix(ct, "video/") {
		ct = videoExt[strings.ToLower(path.Ext(name))]
	}
	title := strings.TrimSpace(o.Title)
	if title == "" {
		title = strings.TrimSuffix(name, path.Ext(name))
	}
	vis := o.Visibility
	if vis != "org" {
		vis = "private"
	}
	kind := o.Kind
	if kind == "" {
		kind = "drive"
	}
	dur, poster := e.probeBlob(ctx, *key, name)
	var id string
	err = e.pool.QueryRow(ctx, `INSERT INTO adrient.videos (org_id, owner_id, title, content_type, size, duration_seconds, thumbnail_data_url, blob_key,
		source_file_id, source_kind, visibility) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::uuid,$10,$11) RETURNING id::text`,
		orgID, userID, title, ct, size, dur, poster, *key, fileID, kind, vis).Scan(&id)
	if err != nil {
		return Info{}, err
	}
	if parent != nil {
		vtt := strings.TrimSuffix(name, path.Ext(name)) + ".vtt"
		var vkey *string
		if e.pool.QueryRow(ctx, `SELECT storage_key FROM adrient.drive_files WHERE org_id=$1::uuid AND parent_id=$2::uuid AND lower(name)=lower($3) AND trashed_at IS NULL`,
			orgID, *parent, vtt).Scan(&vkey) == nil && vkey != nil {
			if b, _, _, err := e.blobs.Get(ctx, *vkey); err == nil {
				data, _ := io.ReadAll(io.LimitReader(b, 2<<20))
				b.Close()
				nk := randKey() + ".vtt"
				if e.blobs.Put(ctx, nk, "text/vtt", int64(len(data)), bytes.NewReader(data)) == nil {
					_, _ = (&CaptionRepository{pool: e.pool}).CreateCaption(ctx, orgID, id, "en", "English", nk)
				}
			}
		}
	}
	return e.One(ctx, orgID, userID, id)
}

// MeetingRecording is a saved Meet recording.
type MeetingRecording struct {
	ID         string     `json:"id"`
	Room       string     `json:"room"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	FileName   string     `json:"file_name"`
	Size       int64      `json:"size"`
	VideoID    string     `json:"video_id"`
}

// Meetings lists the user's saved Meet recordings with the video id once opened.
func (e *Extras) Meetings(ctx context.Context, orgID, userID string) ([]MeetingRecording, error) {
	rows, err := e.pool.Query(ctx, `SELECT rc.id::text, r.name, rc.started_at, rc.finished_at, f.name, f.size_bytes,
		COALESCE((SELECT v.id::text FROM adrient.videos v WHERE v.source_file_id=f.id AND v.trashed_at IS NULL LIMIT 1),'')
		FROM adrient.meet_recordings rc JOIN adrient.meet_rooms r ON r.id=rc.room_id
		JOIN adrient.drive_files f ON f.id=rc.drive_file_id
		WHERE rc.org_id=$1::uuid AND rc.status='saved' AND f.trashed_at IS NULL AND (rc.started_by=$2::uuid OR f.owner_id=$2::uuid)
		ORDER BY rc.started_at DESC LIMIT 100`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MeetingRecording{}
	for rows.Next() {
		var m MeetingRecording
		if err := rows.Scan(&m.ID, &m.Room, &m.StartedAt, &m.FinishedAt, &m.FileName, &m.Size, &m.VideoID); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// OpenMeeting makes a Meet recording watchable as a (private) video.
func (e *Extras) OpenMeeting(ctx context.Context, orgID, userID, recID string) (Info, error) {
	var fileID, room string
	var at time.Time
	err := e.pool.QueryRow(ctx, `SELECT rc.drive_file_id::text, r.name, rc.started_at FROM adrient.meet_recordings rc
		JOIN adrient.meet_rooms r ON r.id=rc.room_id JOIN adrient.drive_files f ON f.id=rc.drive_file_id
		WHERE rc.id=$1::uuid AND rc.org_id=$2::uuid AND rc.status='saved' AND (rc.started_by=$3::uuid OR f.owner_id=$3::uuid)`, recID, orgID, userID).Scan(&fileID, &room, &at)
	if err != nil {
		return Info{}, ErrNotFound
	}
	title := strings.TrimSpace(room)
	if title == "" {
		title = "Meeting"
	}
	title += " · " + at.Format("2 Jan 2006 15:04")
	return e.importAsOwner(ctx, orgID, userID, fileID, ImportOpts{Title: title, Kind: "meet", Visibility: "private"})
}

// importAsOwner imports a file whose Drive owner may differ from the caller
// (the person who started a recording); the video belongs to the caller.
func (e *Extras) importAsOwner(ctx context.Context, orgID, userID, fileID string, o ImportOpts) (Info, error) {
	return e.ImportFromDrive(ctx, orgID, userID, fileID, o)
}

// probeBlob returns the duration and a JPEG poster data URL of a stored video,
// using ffprobe/ffmpeg when installed (both are optional).
func (e *Extras) probeBlob(ctx context.Context, key, name string) (float64, string) {
	ffprobe, perr := exec.LookPath("ffprobe")
	ffmpeg, merr := exec.LookPath("ffmpeg")
	if perr != nil && merr != nil {
		return 0, ""
	}
	body, _, _, err := e.blobs.Get(ctx, key)
	if err != nil {
		return 0, ""
	}
	defer body.Close()
	f, err := os.CreateTemp("", "vx-*"+path.Ext(name))
	if err != nil {
		return 0, ""
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, io.LimitReader(body, 2<<30)); err != nil {
		f.Close()
		return 0, ""
	}
	f.Close()
	return probeFile(ctx, ffprobe, ffmpeg, f.Name())
}

func probeFile(ctx context.Context, ffprobe, ffmpeg, file string) (float64, string) {
	var dur float64
	if ffprobe != "" {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		out, err := exec.CommandContext(cctx, ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", file).Output()
		cancel()
		if err == nil {
			dur, _ = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		}
	}
	poster := ""
	if ffmpeg != "" {
		at := 1.0
		if dur > 0 && dur < 4 {
			at = dur / 2
		} else if dur > 60 {
			at = dur * 0.05
		}
		jpg, err := os.CreateTemp("", "vxp-*.jpg")
		if err == nil {
			jpg.Close()
			defer os.Remove(jpg.Name())
			cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			err = exec.CommandContext(cctx, ffmpeg, "-y", "-v", "error", "-ss", strconv.FormatFloat(at, 'f', 2, 64), "-i", file,
				"-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "5", jpg.Name()).Run()
			cancel()
			if err == nil {
				if b, err := os.ReadFile(jpg.Name()); err == nil && len(b) > 0 && len(b) < 450_000 {
					poster = "data:image/jpeg;base64," + b64(b)
				}
			}
		}
	}
	return dur, poster
}

// ProcessUpload fills in the duration and poster of a freshly uploaded video
// when the browser did not, then marks it ready. It runs in the background.
func (e *Extras) ProcessUpload(orgID, videoID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer func() {
		_, _ = e.pool.Exec(ctx, `UPDATE adrient.videos SET status='ready', updated_at=now() WHERE id=$1::uuid AND status='processing'`, videoID)
	}()
	var key, name string
	var dur float64
	var thumb string
	if e.pool.QueryRow(ctx, `SELECT blob_key, title, duration_seconds, thumbnail_data_url FROM adrient.videos WHERE id=$1::uuid AND org_id=$2::uuid`, videoID, orgID).Scan(&key, &name, &dur, &thumb) != nil {
		return
	}
	if dur > 0 && thumb != "" {
		return
	}
	d, p := e.probeBlob(ctx, key, "x.mp4")
	if dur == 0 && d > 0 {
		_, _ = e.pool.Exec(ctx, `UPDATE adrient.videos SET duration_seconds=$2 WHERE id=$1::uuid`, videoID, d)
	}
	if thumb == "" && p != "" {
		_, _ = e.pool.Exec(ctx, `UPDATE adrient.videos SET thumbnail_data_url=$2 WHERE id=$1::uuid`, videoID, p)
	}
}

// MarkProcessing flags a new upload as processing when a background job will
// complete it (no thumbnail or duration came with the upload).
func (e *Extras) MarkProcessing(ctx context.Context, orgID, videoID string) {
	_, _ = e.pool.Exec(ctx, `UPDATE adrient.videos SET status='processing' WHERE id=$1::uuid AND org_id=$2::uuid`, videoID, orgID)
}

// SetDuration stores the duration measured in the browser when the server had none.
func (e *Extras) SetDuration(ctx context.Context, orgID, userID, id string, secs float64) error {
	if secs <= 0 || secs > 86400*3 {
		return errors.New("duration must be positive")
	}
	_, err := e.pool.Exec(ctx, `UPDATE adrient.videos SET duration_seconds=$4 WHERE id=$1::uuid AND org_id=$2::uuid AND duration_seconds=0 AND
		(owner_id=$3::uuid OR visibility='org')`, id, orgID, userID, secs)
	return err
}

// Groups lists the groups the user belongs to.
func (e *Extras) Groups(ctx context.Context, orgID, userID string) ([]map[string]string, error) {
	rows, err := e.pool.Query(ctx, `SELECT id::text, name FROM adrient.groups WHERE org_id=$1::uuid AND (owner_id=$2::uuid OR member_ids ? $2::text) ORDER BY name`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"id": id, "name": name})
	}
	return out, rows.Err()
}

func randKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "video/" + hex.EncodeToString(b)
}

// --- HTTP -------------------------------------------------------------------

func xJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func xFail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	msg := err.Error()
	for _, k := range []string{"required", "must be", "not a ", "cannot", "too large", "not an "} {
		if strings.Contains(msg, k) {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
	}
	http.Error(w, "server error", http.StatusInternalServerError)
}

func xDecode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(v)
}

// ServeHTTP routes /api/v1/videos/x/*.
func (e *Extras) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	o, ok := auth.OrgFromContext(r.Context())
	if !ok {
		http.Error(w, "no org context", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, xMount), "/"), "/")
	m := r.Method
	q := r.URL.Query()
	okJSON := func() { xJSON(w, 200, map[string]bool{"ok": true}) }
	switch {
	case m == http.MethodGet && len(seg) == 1 && seg[0] == "feed":
		lim, _ := strconv.Atoi(q.Get("limit"))
		l, err := e.Feed(ctx, o.ID, u.ID, FeedFilter{Q: q.Get("q"), Duration: q.Get("duration"), Date: q.Get("date"), Sort: q.Get("sort"),
			Owner: q.Get("owner"), Subs: q.Get("subs") == "1", Source: q.Get("source"), Limit: lim})
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"videos": l})
	case m == http.MethodGet && len(seg) == 2 && seg[0] == "poster":
		b, mime, upd, err := e.PosterBytes(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeContent(w, r, "", upd, bytes.NewReader(b))
	case len(seg) == 2 && seg[0] == "videos" && m == http.MethodGet:
		v, err := e.One(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, v)
	case len(seg) == 2 && seg[0] == "videos" && m == http.MethodPatch:
		var in UpdateInput
		if xDecode(r, &in) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		v, err := e.Update(ctx, o.ID, u.ID, seg[1], in)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, v)
	case len(seg) == 3 && seg[0] == "videos" && seg[2] == "related" && m == http.MethodGet:
		l, err := e.Related(ctx, o.ID, u.ID, seg[1], 14)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"videos": l})
	case len(seg) == 3 && seg[0] == "videos" && seg[2] == "view" && m == http.MethodPost:
		var in struct {
			Seconds float64 `json:"seconds"`
		}
		_ = xDecode(r, &in)
		if err := e.RecordView(ctx, o.ID, u.ID, seg[1], in.Seconds); err != nil {
			xFail(w, err)
			return
		}
		okJSON()
	case len(seg) == 3 && seg[0] == "videos" && seg[2] == "duration" && m == http.MethodPost:
		var in struct {
			Seconds float64 `json:"seconds"`
		}
		if xDecode(r, &in) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := e.SetDuration(ctx, o.ID, u.ID, seg[1], in.Seconds); err != nil {
			xFail(w, err)
			return
		}
		okJSON()
	case len(seg) == 3 && seg[0] == "videos" && seg[2] == "stats" && m == http.MethodGet:
		s, err := e.Stats(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, s)
	case len(seg) == 3 && seg[0] == "videos" && seg[2] == "like" && (m == http.MethodPut || m == http.MethodDelete):
		if err := e.SetLike(ctx, o.ID, u.ID, seg[1], m == http.MethodPut); err != nil {
			xFail(w, err)
			return
		}
		okJSON()
	case len(seg) == 3 && seg[0] == "videos" && seg[2] == "comments" && m == http.MethodGet:
		c, err := e.Comments(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"comments": c})
	case len(seg) == 3 && seg[0] == "videos" && seg[2] == "comments" && m == http.MethodPost:
		var in struct {
			Body     string `json:"body"`
			ParentID string `json:"parent_id"`
		}
		if xDecode(r, &in) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		id, err := e.AddComment(ctx, o.ID, u.ID, seg[1], in.ParentID, in.Body)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]string{"id": id})
	case len(seg) == 3 && seg[0] == "comments" && seg[2] == "like" && m == http.MethodPut:
		on, n, err := e.ToggleCommentLike(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"liked": on, "likes": n})
	case len(seg) == 2 && seg[0] == "comments" && m == http.MethodDelete:
		if err := e.DeleteComment(ctx, o.ID, u.ID, seg[1]); err != nil {
			xFail(w, err)
			return
		}
		okJSON()
	case len(seg) == 2 && seg[0] == "channels" && m == http.MethodGet:
		c, err := e.Channel(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, c)
	case len(seg) == 3 && seg[0] == "channels" && seg[2] == "subscribe" && (m == http.MethodPut || m == http.MethodDelete):
		if err := e.Subscribe(ctx, o.ID, u.ID, seg[1], m == http.MethodPut); err != nil {
			xFail(w, err)
			return
		}
		okJSON()
	case len(seg) == 1 && seg[0] == "subscriptions" && m == http.MethodGet:
		c, err := e.Subscriptions(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"channels": c})
	case len(seg) == 1 && seg[0] == "watch-later" && m == http.MethodGet:
		l, err := e.WatchLater(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"videos": l})
	case len(seg) == 2 && seg[0] == "watch-later" && (m == http.MethodPut || m == http.MethodDelete):
		if err := e.SetWatchLater(ctx, o.ID, u.ID, seg[1], m == http.MethodPut); err != nil {
			xFail(w, err)
			return
		}
		okJSON()
	case len(seg) == 1 && seg[0] == "history" && m == http.MethodGet:
		l, err := e.History(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"videos": l})
	case len(seg) == 1 && seg[0] == "history" && m == http.MethodDelete:
		if err := e.ClearHistory(ctx, u.ID); err != nil {
			xFail(w, err)
			return
		}
		okJSON()
	case len(seg) == 1 && seg[0] == "meetings" && m == http.MethodGet:
		l, err := e.Meetings(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"recordings": l})
	case len(seg) == 3 && seg[0] == "meetings" && seg[2] == "open" && m == http.MethodPost:
		v, err := e.OpenMeeting(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, v)
	case len(seg) == 1 && seg[0] == "drive-files" && m == http.MethodGet:
		l, err := e.DriveVideoFiles(ctx, o.ID, u.ID, q.Get("q"))
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"files": l})
	case len(seg) == 1 && seg[0] == "import-drive" && m == http.MethodPost:
		var in struct {
			FileID     string `json:"file_id"`
			Title      string `json:"title"`
			Visibility string `json:"visibility"`
		}
		if xDecode(r, &in) != nil || in.FileID == "" {
			http.Error(w, "file_id required", http.StatusBadRequest)
			return
		}
		v, err := e.ImportFromDrive(ctx, o.ID, u.ID, in.FileID, ImportOpts{Title: in.Title, Visibility: in.Visibility})
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, v)
	case len(seg) == 1 && seg[0] == "groups" && m == http.MethodGet:
		g, err := e.Groups(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"groups": g})
	default:
		http.NotFound(w, r)
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// decodeDataURL splits "data:<mime>;base64,<payload>" into bytes and mime.
func decodeDataURL(du string, upd time.Time) ([]byte, string, time.Time, error) {
	if !strings.HasPrefix(du, "data:") {
		return nil, "", upd, ErrNotFound
	}
	comma := strings.IndexByte(du, ',')
	if comma < 0 {
		return nil, "", upd, ErrNotFound
	}
	mime := strings.TrimSuffix(du[5:comma], ";base64")
	if !strings.HasPrefix(mime, "image/") || mime == "image/svg+xml" {
		return nil, "", upd, ErrNotFound
	}
	b, err := base64.StdEncoding.DecodeString(du[comma+1:])
	if err != nil {
		return nil, "", upd, ErrNotFound
	}
	return b, mime, upd, nil
}

// visibleOnly drops videos the user may not see (other people's private or
// group-only videos) from an org-wide list.
func (r *Repository) visibleOnly(ctx context.Context, orgID, userID string, in []Video) []Video {
	if len(in) == 0 {
		return in
	}
	ids := make([]string, len(in))
	for i, v := range in {
		ids[i] = v.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT v.id::text FROM adrient.videos v WHERE v.id = ANY($3::uuid[]) AND `+visibleClause, orgID, userID, ids)
	allowed := map[string]bool{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				allowed[id] = true
			}
		}
	}
	out := in[:0:0]
	for _, v := range in {
		if allowed[v.ID] || (err != nil && v.OwnerID == userID) {
			out = append(out, v)
		}
	}
	return out
}
