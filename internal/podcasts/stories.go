package podcasts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors mapped to HTTP statuses by the handler.
var (
	ErrInvalid   = errors.New("invalid")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrLimit     = errors.New("limit")
)

func invalid(f string, a ...any) error { return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(f, a...)) }
func forbidden(f string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrForbidden, fmt.Sprintf(f, a...))
}
func conflict(f string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(f, a...))
}
func limit(f string, a ...any) error { return fmt.Errorf("%w: %s", ErrLimit, fmt.Sprintf(f, a...)) }

// BlobStore is the object storage the stories use for covers and generated audio
// (the same S3-compatible store as Drive; *drive.Blobs satisfies it).
type BlobStore interface {
	Put(ctx context.Context, key, mimeType string, size int64, body io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
	Delete(ctx context.Context, key string) error
}

// Notifier delivers a notification-center entry.
type Notifier func(ctx context.Context, orgID, userID, actorID, typ, title, body, url string)

// AuditFunc records an audit entry for work done outside a request.
type AuditFunc func(ctx context.Context, orgID, actorID, action, resourceID string, detail map[string]any)

// Stories is the service behind the Stories app.
type Stories struct {
	Pool   *pgxpool.Pool
	Blobs  BlobStore
	Notify Notifier
	Audit  AuditFunc
	// Secret seals stored voice-provider API keys (ADRIENT_SECRETS_KEY). Without
	// it, keys can only be read from an environment variable (key_env).
	Secret string
	Now    func() time.Time
	// Workers is the number of audio jobs run at once (default 2).
	Workers int

	kick     chan struct{}
	stopOnce sync.Once
	stop     chan struct{}
}

// NewStories builds the service. Call Start to run the audio worker and the scheduler.
func NewStories(pool *pgxpool.Pool, blobs BlobStore) *Stories {
	return &Stories{Pool: pool, Blobs: blobs, Now: time.Now, kick: make(chan struct{}, 1), stop: make(chan struct{})}
}

func (s *Stories) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Viewer is the person making a request.
type Viewer struct {
	OrgID, UserID string
	Admin         bool
	Groups, Depts []string
}

// LoadViewer reads the viewer's group and department ids (the department and all its parents).
func (s *Stories) LoadViewer(ctx context.Context, org, user string, admin bool) (Viewer, error) {
	v := Viewer{OrgID: org, UserID: user, Admin: admin, Groups: []string{}, Depts: []string{}}
	rows, err := s.Pool.Query(ctx, `SELECT group_id::text FROM adrient.group_members WHERE org_id=$1 AND user_id=$2::uuid`, org, user)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err == nil {
			v.Groups = append(v.Groups, g)
		}
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `
WITH RECURSIVE d(id) AS (
    SELECT department_id FROM adrient.users WHERE id=$2::uuid AND org_id=$1 AND department_id IS NOT NULL
    UNION
    SELECT p.parent_id FROM adrient.departments p JOIN d ON p.id = d.id WHERE p.parent_id IS NOT NULL
) SELECT id::text FROM d`, org, user)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err == nil {
			v.Depts = append(v.Depts, g)
		}
	}
	rows.Close()
	return v, nil
}

// visible is the SQL condition "the viewer may open show alias s". It uses $1 org,
// $2 user, $3 groups, $4 departments, so every query using it starts with viewerArgs.
const visible = `(s.org_id=$1::uuid AND (s.author_id=$2::uuid OR s.audience='everyone'
	OR (s.audience='group' AND s.audience_id = ANY($3::uuid[]))
	OR (s.audience='department' AND s.audience_id = ANY($4::uuid[]))))`

func (v Viewer) args(more ...any) []any {
	return append([]any{v.OrgID, v.UserID, v.Groups, v.Depts}, more...)
}

// ---- JSON shapes -----------------------------------------------------------

// Person is an author as shown to listeners.
type Person struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Show is a story or series.
type Show struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Genre         string    `json:"genre"`
	Language      string    `json:"language"`
	CoverURL      string    `json:"cover_url,omitempty"`
	CoverStyle    string    `json:"cover_style"`
	Author        Person    `json:"author"`
	Audience      string    `json:"audience"`
	AudienceID    string    `json:"audience_id,omitempty"`
	CommentsOn    bool      `json:"comments_on"`
	AllowDownload bool      `json:"allow_download"`
	Provider      string    `json:"provider"` // "default", "browser" or a provider id
	Voice         string    `json:"voice"`
	VoiceMap      Voices    `json:"voice_map"`
	Speed         float64   `json:"speed"`
	Pitch         float64   `json:"pitch"`
	Episodes      int       `json:"episodes"`
	Published     int       `json:"published"`
	TotalMs       int       `json:"total_ms"`
	Following     bool      `json:"following"`
	InLibrary     bool      `json:"in_library"`
	Mine          bool      `json:"mine"`
	Followers     int       `json:"followers"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// Voices maps a character name to a voice id ("" is the narrator).
type Voices map[string]string

// Episode is one episode of a show.
type Episode struct {
	ID          string     `json:"id"`
	ShowID      string     `json:"show_id"`
	ShowTitle   string     `json:"show_title"`
	Position    int        `json:"position"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Status      string     `json:"status"`
	PublishAt   *time.Time `json:"publish_at,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	AudioStatus string     `json:"audio_status"`
	AudioError  string     `json:"audio_error,omitempty"`
	HasAudio    bool       `json:"has_audio"`
	DurationMs  int        `json:"duration_ms"`
	Likes       int        `json:"likes"`
	Liked       bool       `json:"liked"`
	Saved       bool       `json:"saved"`
	Comments    int        `json:"comments"`
	PositionMs  int        `json:"position_ms"`
	ProgressDur int        `json:"progress_duration_ms"`
	Completed   bool       `json:"completed"`
	Author      Person     `json:"author"`
	CoverStyle  string     `json:"cover_style"`
	CoverURL    string     `json:"cover_url,omitempty"`
	Genre       string     `json:"genre"`
	Language    string     `json:"language"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

const epSelect = `e.id::text, e.show_id::text, s.title, e.position, e.title, e.summary, e.status, e.publish_at, e.published_at,
	e.audio_status, e.audio_error, e.audio_ms, e.est_ms,
	(SELECT count(*) FROM adrient.story_likes l WHERE l.episode_id=e.id),
	EXISTS(SELECT 1 FROM adrient.story_likes l WHERE l.episode_id=e.id AND l.user_id=$2::uuid),
	EXISTS(SELECT 1 FROM adrient.story_library b WHERE b.user_id=$2::uuid AND b.kind='episode' AND b.target_id=e.id),
	(SELECT count(*) FROM adrient.story_comments c WHERE c.episode_id=e.id AND c.status='visible'),
	COALESCE(p.position_ms,0), COALESCE(p.duration_ms,0), COALESCE(p.completed,false),
	s.author_id::text, u.display_name, s.cover_style, s.cover_blob<>'', s.genre, s.language, e.updated_at`

const epFrom = `FROM adrient.story_episodes e JOIN adrient.story_shows s ON s.id=e.show_id JOIN adrient.users u ON u.id=s.author_id
	LEFT JOIN adrient.story_progress p ON p.episode_id=e.id AND p.user_id=$2::uuid`

func scanEpisode(row pgx.Row) (Episode, error) {
	var e Episode
	var audioMs, estMs int
	var hasCover bool
	err := row.Scan(&e.ID, &e.ShowID, &e.ShowTitle, &e.Position, &e.Title, &e.Summary, &e.Status, &e.PublishAt, &e.PublishedAt,
		&e.AudioStatus, &e.AudioError, &audioMs, &estMs, &e.Likes, &e.Liked, &e.Saved, &e.Comments,
		&e.PositionMs, &e.ProgressDur, &e.Completed, &e.Author.ID, &e.Author.Name, &e.CoverStyle, &hasCover, &e.Genre, &e.Language, &e.UpdatedAt)
	if err != nil {
		return e, err
	}
	e.HasAudio = e.AudioStatus == "ready"
	e.DurationMs = estMs
	if e.HasAudio && audioMs > 0 {
		e.DurationMs = audioMs
	}
	if hasCover {
		e.CoverURL = "/api/v1/stories/shows/" + e.ShowID + "/cover?v=" + fmt.Sprint(e.UpdatedAt.Unix())
	}
	return e, nil
}

func (s *Stories) queryEpisodes(ctx context.Context, sql string, args ...any) ([]Episode, error) {
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Episode{}
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

const showSelect = `s.id::text, s.title, s.description, s.genre, s.language, s.cover_blob<>'', s.cover_style, s.author_id::text, u.display_name,
	s.audience, COALESCE(s.audience_id::text,''), s.comments_on, s.allow_download, s.voice_provider::text, s.voice, s.voice_map, s.speed, s.pitch,
	(SELECT count(*) FROM adrient.story_episodes e WHERE e.show_id=s.id AND (e.status='published' OR s.author_id=$2::uuid)),
	(SELECT count(*) FROM adrient.story_episodes e WHERE e.show_id=s.id AND e.status='published'),
	COALESCE((SELECT sum(CASE WHEN e.audio_status='ready' AND e.audio_ms>0 THEN e.audio_ms ELSE e.est_ms END) FROM adrient.story_episodes e WHERE e.show_id=s.id AND (e.status='published' OR s.author_id=$2::uuid)),0),
	EXISTS(SELECT 1 FROM adrient.story_follows f WHERE f.user_id=$2::uuid AND f.kind='show' AND f.target_id=s.id),
	EXISTS(SELECT 1 FROM adrient.story_library b WHERE b.user_id=$2::uuid AND b.kind='show' AND b.target_id=s.id),
	(SELECT count(*) FROM adrient.story_follows f WHERE f.kind='show' AND f.target_id=s.id),
	s.updated_at, s.created_at`

const showFrom = `FROM adrient.story_shows s JOIN adrient.users u ON u.id=s.author_id`

const zeroUUID = "00000000-0000-0000-0000-000000000000"

func scanShow(row pgx.Row, viewer string) (Show, error) {
	var sh Show
	var hasCover bool
	var prov *string
	var vm map[string]string
	var totalMs int64
	err := row.Scan(&sh.ID, &sh.Title, &sh.Description, &sh.Genre, &sh.Language, &hasCover, &sh.CoverStyle, &sh.Author.ID, &sh.Author.Name,
		&sh.Audience, &sh.AudienceID, &sh.CommentsOn, &sh.AllowDownload, &prov, &sh.Voice, &vm, &sh.Speed, &sh.Pitch,
		&sh.Episodes, &sh.Published, &totalMs, &sh.Following, &sh.InLibrary, &sh.Followers, &sh.UpdatedAt, &sh.CreatedAt)
	if err != nil {
		return sh, err
	}
	sh.TotalMs = int(totalMs)
	sh.VoiceMap = Voices(vm)
	if sh.VoiceMap == nil {
		sh.VoiceMap = Voices{}
	}
	sh.Provider = providerName(prov)
	if hasCover {
		sh.CoverURL = "/api/v1/stories/shows/" + sh.ID + "/cover?v=" + fmt.Sprint(sh.UpdatedAt.Unix())
	}
	sh.Mine = sh.Author.ID == viewer
	return sh, nil
}

func providerName(p *string) string {
	switch {
	case p == nil || *p == "":
		return "default"
	case *p == zeroUUID:
		return "browser"
	}
	return *p
}

func providerArg(name string) (any, error) {
	switch name {
	case "", "default":
		return nil, nil
	case "browser":
		return zeroUUID, nil
	}
	if !isUUID(name) {
		return nil, invalid("voice provider is not valid")
	}
	return name, nil
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

func cleanText(s string, max int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

func (s *Stories) log(msg string, err error) {
	if err != nil {
		slog.Warn("stories: "+msg, "err", err)
	}
}
