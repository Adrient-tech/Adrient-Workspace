package podcasts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// GenreCount is one genre chip.
type GenreCount struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

// Home is everything the landing page shows.
type Home struct {
	Continue  []Episode    `json:"continue"`
	New       []Episode    `json:"new"`
	Following []Episode    `json:"following"`
	Trending  []Show       `json:"trending"`
	Genres    []GenreCount `json:"genres"`
	Mine      int          `json:"my_stories"`
}

// Home builds the landing page for the viewer.
func (s *Stories) Home(ctx context.Context, v Viewer) (Home, error) {
	h := Home{Genres: []GenreCount{}}
	vis := ` AND ` + visible + ` AND e.status='published'`
	var err error
	h.Continue, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE p.user_id IS NOT NULL AND p.completed=false AND p.position_ms>0`+vis+`
		ORDER BY p.updated_at DESC LIMIT 12`, v.args()...)
	if err != nil {
		return h, err
	}
	h.New, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE e.published_at > now() - interval '7 days'`+vis+`
		ORDER BY e.published_at DESC LIMIT 12`, v.args()...)
	if err != nil {
		return h, err
	}
	if len(h.New) < 4 {
		h.New, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE true`+vis+` ORDER BY e.published_at DESC NULLS LAST LIMIT 12`, v.args()...)
		if err != nil {
			return h, err
		}
	}
	h.Following, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE e.published_at > now() - interval '30 days'
		AND (EXISTS(SELECT 1 FROM adrient.story_follows f WHERE f.user_id=$2::uuid AND f.kind='show' AND f.target_id=s.id)
		  OR EXISTS(SELECT 1 FROM adrient.story_follows f WHERE f.user_id=$2::uuid AND f.kind='author' AND f.target_id=s.author_id))`+vis+`
		ORDER BY e.published_at DESC LIMIT 12`, v.args()...)
	if err != nil {
		return h, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+showSelect+` `+showFrom+` WHERE `+visible+` AND EXISTS(SELECT 1 FROM adrient.story_episodes e WHERE e.show_id=s.id AND e.status='published')
		ORDER BY (SELECT count(*) FROM adrient.story_plays p JOIN adrient.story_episodes e ON e.id=p.episode_id WHERE e.show_id=s.id AND p.created_at > now() - interval '14 days')
		       + 2*(SELECT count(*) FROM adrient.story_likes l JOIN adrient.story_episodes e ON e.id=l.episode_id WHERE e.show_id=s.id) DESC, s.updated_at DESC LIMIT 12`, v.args()...)
	if err != nil {
		return h, err
	}
	h.Trending = []Show{}
	for rows.Next() {
		sh, err := scanShow(rows, v.UserID)
		if err != nil {
			rows.Close()
			return h, err
		}
		h.Trending = append(h.Trending, sh)
	}
	rows.Close()
	grows, err := s.Pool.Query(ctx, `SELECT s.genre, count(*) FROM adrient.story_shows s WHERE `+visible+`
		AND EXISTS(SELECT 1 FROM adrient.story_episodes e WHERE e.show_id=s.id AND e.status='published') GROUP BY s.genre ORDER BY count(*) DESC, s.genre`, v.args()...)
	if err != nil {
		return h, err
	}
	for grows.Next() {
		var g GenreCount
		if grows.Scan(&g.Genre, &g.Count) == nil {
			h.Genres = append(h.Genres, g)
		}
	}
	grows.Close()
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.story_shows WHERE org_id=$1::uuid AND author_id=$2::uuid`, v.OrgID, v.UserID).Scan(&h.Mine)
	return h, nil
}

// Search finds shows and episodes by title, description or text.
type SearchResult struct {
	Shows    []Show    `json:"shows"`
	Episodes []Episode `json:"episodes"`
}

// Search runs a text search the viewer is allowed to see.
func (s *Stories) Search(ctx context.Context, v Viewer, q string) (SearchResult, error) {
	r := SearchResult{Shows: []Show{}, Episodes: []Episode{}}
	q = strings.TrimSpace(q)
	if q == "" {
		return r, nil
	}
	var err error
	r.Shows, err = s.ListShows(ctx, v, ShowFilter{Q: q, Limit: 20})
	if err != nil {
		return r, err
	}
	r.Episodes, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE `+visible+` AND e.status='published'
		AND (lower(e.title) LIKE $5 ESCAPE '\' OR lower(e.summary) LIKE $5 ESCAPE '\' OR lower(e.body_text) LIKE $5 ESCAPE '\')
		ORDER BY e.published_at DESC NULLS LAST LIMIT 30`, v.args(likePattern(q))...)
	return r, err
}

// SearchHit is one result for the launcher search.
type SearchHit struct{ ID, Title, Snippet, URL string }

// GlobalSearch serves the launcher search (global search registry).
func (s *Stories) GlobalSearch(ctx context.Context, org, user, pattern string, limit int) ([]SearchHit, error) {
	v, err := s.LoadViewer(ctx, org, user, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT e.id::text, e.title, s.title || ' · ' || u.display_name FROM adrient.story_episodes e
		JOIN adrient.story_shows s ON s.id=e.show_id JOIN adrient.users u ON u.id=s.author_id
		WHERE `+visible+` AND e.status='published' AND (lower(e.title) LIKE $5 ESCAPE '\' OR lower(s.title) LIKE $5 ESCAPE '\' OR lower(e.summary) LIKE $5 ESCAPE '\')
		ORDER BY e.published_at DESC NULLS LAST LIMIT $6`, v.args(pattern, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchHit
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.ID, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		h.URL = "/podcasts/episode/" + h.ID
		out = append(out, h)
	}
	return out, rows.Err()
}

// ---- listening ----------------------------------------------------------------------

// canListen checks an episode is published (or yours) and visible; it returns the show id.
func (s *Stories) canListen(ctx context.Context, v Viewer, epID string) (showID string, err error) {
	if !isUUID(epID) {
		return "", ErrNotFound
	}
	err = s.Pool.QueryRow(ctx, `SELECT e.show_id::text FROM adrient.story_episodes e JOIN adrient.story_shows s ON s.id=e.show_id
		WHERE e.id=$5::uuid AND (e.status='published' OR s.author_id=$2::uuid) AND `+visible, v.args(epID)...).Scan(&showID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return showID, err
}

// Progress is the listener's place in an episode.
type Progress struct {
	PositionMs int  `json:"position_ms"`
	DurationMs int  `json:"duration_ms"`
	Completed  bool `json:"completed"`
}

// SaveProgress remembers where the viewer is. Reaching 95% counts as played.
func (s *Stories) SaveProgress(ctx context.Context, v Viewer, epID string, p Progress) error {
	showID, err := s.canListen(ctx, v, epID)
	if err != nil {
		return err
	}
	if p.PositionMs < 0 || p.DurationMs < 0 || p.PositionMs > 1<<30 || p.DurationMs > 1<<30 {
		return invalid("position is out of range")
	}
	if p.DurationMs > 0 && p.PositionMs*100 >= p.DurationMs*95 {
		p.Completed = true
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO adrient.story_progress (org_id, user_id, episode_id, show_id, position_ms, duration_ms, completed)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7)
		ON CONFLICT (user_id, episode_id) DO UPDATE SET position_ms=$5, duration_ms=GREATEST($6, adrient.story_progress.duration_ms),
		  completed = adrient.story_progress.completed OR $7, updated_at=now()`,
		v.OrgID, v.UserID, epID, showID, p.PositionMs, p.DurationMs, p.Completed)
	return err
}

// MarkPlayed sets or clears the played mark.
func (s *Stories) MarkPlayed(ctx context.Context, v Viewer, epID string, played bool) error {
	showID, err := s.canListen(ctx, v, epID)
	if err != nil {
		return err
	}
	pos := "0"
	if played {
		pos = "adrient.story_progress.duration_ms"
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO adrient.story_progress (org_id, user_id, episode_id, show_id, position_ms, duration_ms, completed)
		VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,0,0,$5)
		ON CONFLICT (user_id, episode_id) DO UPDATE SET completed=$5, position_ms=`+pos+`, updated_at=now()`, v.OrgID, v.UserID, epID, showID, played)
	return err
}

// RecordPlay counts one start of listening (at most one per viewer and episode every 30 minutes).
func (s *Stories) RecordPlay(ctx context.Context, v Viewer, epID string) error {
	if _, err := s.canListen(ctx, v, epID); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.story_plays (org_id, episode_id, user_id) SELECT $1::uuid,$2::uuid,$3::uuid
		WHERE NOT EXISTS (SELECT 1 FROM adrient.story_plays WHERE episode_id=$2::uuid AND user_id=$3::uuid AND created_at > now() - interval '30 minutes')`, v.OrgID, epID, v.UserID)
	return err
}

// SetLike likes or unlikes an episode.
func (s *Stories) SetLike(ctx context.Context, v Viewer, epID string, on bool) error {
	if _, err := s.canListen(ctx, v, epID); err != nil {
		return err
	}
	if on {
		_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.story_likes (org_id, user_id, episode_id) VALUES ($1::uuid,$2::uuid,$3::uuid) ON CONFLICT DO NOTHING`, v.OrgID, v.UserID, epID)
		return err
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_likes WHERE user_id=$1::uuid AND episode_id=$2::uuid`, v.UserID, epID)
	return err
}

// SetLibrary saves or removes a show or an episode in the viewer's library.
func (s *Stories) SetLibrary(ctx context.Context, v Viewer, kind, id string, on bool) error {
	switch kind {
	case "episode":
		if _, err := s.canListen(ctx, v, id); err != nil {
			return err
		}
	case "show":
		if _, err := s.GetShow(ctx, v, id); err != nil {
			return err
		}
	default:
		return invalid("kind must be show or episode")
	}
	if on {
		_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.story_library (org_id, user_id, kind, target_id) VALUES ($1::uuid,$2::uuid,$3,$4::uuid) ON CONFLICT DO NOTHING`, v.OrgID, v.UserID, kind, id)
		return err
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_library WHERE user_id=$1::uuid AND kind=$2 AND target_id=$3::uuid`, v.UserID, kind, id)
	return err
}

// Library is what the viewer saved.
type Library struct {
	Episodes []Episode `json:"episodes"`
	Shows    []Show    `json:"shows"`
	Liked    []Episode `json:"liked"`
	History  []Episode `json:"history"`
}

// MyLibrary lists saved episodes and shows, liked episodes and listening history.
func (s *Stories) MyLibrary(ctx context.Context, v Viewer) (Library, error) {
	var l Library
	var err error
	vis := ` AND ` + visible + ` AND e.status='published'`
	l.Episodes, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` JOIN adrient.story_library b ON b.target_id=e.id AND b.kind='episode' AND b.user_id=$2::uuid
		WHERE true`+vis+` ORDER BY b.created_at DESC LIMIT 100`, v.args()...)
	if err != nil {
		return l, err
	}
	l.Liked, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` JOIN adrient.story_likes k ON k.episode_id=e.id AND k.user_id=$2::uuid
		WHERE true`+vis+` ORDER BY k.created_at DESC LIMIT 100`, v.args()...)
	if err != nil {
		return l, err
	}
	l.History, err = s.queryEpisodes(ctx, `SELECT `+epSelect+` `+epFrom+` WHERE p.user_id IS NOT NULL`+vis+` ORDER BY p.updated_at DESC LIMIT 50`, v.args()...)
	if err != nil {
		return l, err
	}
	l.Shows, err = s.ListShows(ctx, v, ShowFilter{Library: true})
	return l, err
}

// SetFollow follows or unfollows a show or an author.
func (s *Stories) SetFollow(ctx context.Context, v Viewer, kind, id string, on bool) error {
	switch kind {
	case "show":
		if _, err := s.GetShow(ctx, v, id); err != nil {
			return err
		}
	case "author":
		var n int
		if !isUUID(id) {
			return ErrNotFound
		}
		if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE id=$1::uuid AND org_id=$2::uuid`, id, v.OrgID).Scan(&n); err != nil || n == 0 {
			return ErrNotFound
		}
	default:
		return invalid("kind must be show or author")
	}
	if on {
		_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.story_follows (org_id, user_id, kind, target_id) VALUES ($1::uuid,$2::uuid,$3,$4::uuid) ON CONFLICT DO NOTHING`, v.OrgID, v.UserID, kind, id)
		return err
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.story_follows WHERE user_id=$1::uuid AND kind=$2 AND target_id=$3::uuid`, v.UserID, kind, id)
	return err
}

// FollowedAuthors lists the author ids the viewer follows.
func (s *Stories) FollowedAuthors(ctx context.Context, v Viewer) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT target_id::text FROM adrient.story_follows WHERE user_id=$1::uuid AND kind='author'`, v.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

// ---- comments ---------------------------------------------------------------------

// Comment is one comment on an episode.
type Comment struct {
	ID        string    `json:"id"`
	EpisodeID string    `json:"episode_id"`
	Author    Person    `json:"author"`
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	Mine      bool      `json:"mine"`
	Reports   int       `json:"reports"`
	Reported  bool      `json:"reported"` // reported by the viewer
	CreatedAt time.Time `json:"created_at"`
	Episode   string    `json:"episode,omitempty"`
}

// isModerator: the author of the show, or an org admin.
func (s *Stories) isModerator(ctx context.Context, v Viewer, epID string) (bool, error) {
	if v.Admin {
		return true, nil
	}
	var author string
	err := s.Pool.QueryRow(ctx, `SELECT s.author_id::text FROM adrient.story_episodes e JOIN adrient.story_shows s ON s.id=e.show_id WHERE e.id=$1::uuid AND e.org_id=$2::uuid`, epID, v.OrgID).Scan(&author)
	if err != nil {
		return false, ErrNotFound
	}
	return author == v.UserID, nil
}

// ListComments returns visible comments (moderators also see hidden ones).
func (s *Stories) ListComments(ctx context.Context, v Viewer, epID string) ([]Comment, error) {
	if _, err := s.canListen(ctx, v, epID); err != nil {
		return nil, err
	}
	mod, _ := s.isModerator(ctx, v, epID)
	rows, err := s.Pool.Query(ctx, `SELECT c.id::text, c.episode_id::text, c.user_id::text, u.display_name, c.body, c.status, c.created_at,
		(SELECT count(*) FROM adrient.story_reports r WHERE r.comment_id=c.id AND NOT r.resolved),
		EXISTS(SELECT 1 FROM adrient.story_reports r WHERE r.comment_id=c.id AND r.reporter_id=$2::uuid)
		FROM adrient.story_comments c JOIN adrient.users u ON u.id=c.user_id
		WHERE c.episode_id=$1::uuid AND (c.status='visible' OR $3 OR c.user_id=$2::uuid) ORDER BY c.created_at`, epID, v.UserID, mod)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.EpisodeID, &c.Author.ID, &c.Author.Name, &c.Body, &c.Status, &c.CreatedAt, &c.Reports, &c.Reported); err != nil {
			return nil, err
		}
		c.Mine = c.Author.ID == v.UserID
		if !mod {
			c.Reports = 0
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddComment posts a comment when the show allows comments.
func (s *Stories) AddComment(ctx context.Context, v Viewer, epID, body string) (Comment, error) {
	if _, err := s.canListen(ctx, v, epID); err != nil {
		return Comment{}, err
	}
	body = cleanText(body, 2000)
	if body == "" {
		return Comment{}, invalid("write a comment first")
	}
	var on bool
	_ = s.Pool.QueryRow(ctx, `SELECT s.comments_on FROM adrient.story_episodes e JOIN adrient.story_shows s ON s.id=e.show_id WHERE e.id=$1::uuid`, epID).Scan(&on)
	if !on {
		return Comment{}, forbidden("comments are turned off for this story")
	}
	var recent int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.story_comments WHERE user_id=$1::uuid AND created_at > now() - interval '1 minute'`, v.UserID).Scan(&recent)
	if recent >= 8 {
		return Comment{}, limit("slow down, you are commenting too fast")
	}
	var id string
	if err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.story_comments (org_id, episode_id, user_id, body) VALUES ($1::uuid,$2::uuid,$3::uuid,$4) RETURNING id::text`, v.OrgID, epID, v.UserID, body).Scan(&id); err != nil {
		return Comment{}, err
	}
	list, err := s.ListComments(ctx, v, epID)
	if err != nil {
		return Comment{}, err
	}
	for _, c := range list {
		if c.ID == id {
			// tell the author
			if s.Notify != nil {
				var author, title string
				if err := s.Pool.QueryRow(ctx, `SELECT s.author_id::text, e.title FROM adrient.story_episodes e JOIN adrient.story_shows s ON s.id=e.show_id WHERE e.id=$1::uuid`, epID).Scan(&author, &title); err == nil && author != v.UserID {
					s.Notify(ctx, v.OrgID, author, v.UserID, "story_comment", "New comment on "+title, c.Author.Name+": "+cleanText(body, 120), "/podcasts/episode/"+epID)
				}
			}
			return c, nil
		}
	}
	return Comment{}, errors.New("comment not found after saving")
}

func (s *Stories) commentEpisode(ctx context.Context, v Viewer, id string) (epID string, owner string, err error) {
	if !isUUID(id) {
		return "", "", ErrNotFound
	}
	err = s.Pool.QueryRow(ctx, `SELECT episode_id::text, user_id::text FROM adrient.story_comments WHERE id=$1::uuid AND org_id=$2::uuid`, id, v.OrgID).Scan(&epID, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err == nil {
		if _, err = s.canListen(ctx, v, epID); err != nil {
			return "", "", err
		}
	}
	return
}

// ReportComment flags a comment for the moderators.
func (s *Stories) ReportComment(ctx context.Context, v Viewer, id, reason string) error {
	if _, _, err := s.commentEpisode(ctx, v, id); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.story_reports (org_id, comment_id, reporter_id, reason) VALUES ($1::uuid,$2::uuid,$3::uuid,$4)
		ON CONFLICT (comment_id, reporter_id) DO UPDATE SET reason=$4, resolved=false`, v.OrgID, id, v.UserID, cleanText(reason, 300))
	return err
}

// ModerateComment hides, shows or deletes a comment. Authors of the show and
// admins may moderate; commenters may delete their own.
func (s *Stories) ModerateComment(ctx context.Context, v Viewer, id, action string) error {
	epID, owner, err := s.commentEpisode(ctx, v, id)
	if err != nil {
		return err
	}
	mod, err := s.isModerator(ctx, v, epID)
	if err != nil {
		return err
	}
	switch action {
	case "delete":
		if !mod && owner != v.UserID {
			return forbidden("you cannot delete this comment")
		}
		_, err = s.Pool.Exec(ctx, `DELETE FROM adrient.story_comments WHERE id=$1::uuid`, id)
	case "hide", "show", "dismiss":
		if !mod {
			return forbidden("only the author or an administrator can moderate comments")
		}
		if action == "dismiss" {
			_, err = s.Pool.Exec(ctx, `UPDATE adrient.story_reports SET resolved=true WHERE comment_id=$1::uuid`, id)
		} else if action == "hide" {
			_, err = s.Pool.Exec(ctx, `UPDATE adrient.story_comments SET status='hidden', hidden_by=$2::uuid WHERE id=$1::uuid`, id, v.UserID)
			_, _ = s.Pool.Exec(ctx, `UPDATE adrient.story_reports SET resolved=true WHERE comment_id=$1::uuid`, id)
		} else {
			_, err = s.Pool.Exec(ctx, `UPDATE adrient.story_comments SET status='visible', hidden_by=NULL WHERE id=$1::uuid`, id)
		}
	default:
		return invalid("unknown action")
	}
	return err
}

// ReportedComments is the moderation queue: reported comments on the viewer's
// stories (every story for an admin).
func (s *Stories) ReportedComments(ctx context.Context, v Viewer) ([]Comment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT c.id::text, c.episode_id::text, c.user_id::text, u.display_name, c.body, c.status, c.created_at, e.title,
		(SELECT count(*) FROM adrient.story_reports r WHERE r.comment_id=c.id AND NOT r.resolved)
		FROM adrient.story_comments c JOIN adrient.users u ON u.id=c.user_id JOIN adrient.story_episodes e ON e.id=c.episode_id JOIN adrient.story_shows s ON s.id=e.show_id
		WHERE c.org_id=$1::uuid AND ($3 OR s.author_id=$2::uuid)
		  AND EXISTS(SELECT 1 FROM adrient.story_reports r WHERE r.comment_id=c.id AND NOT r.resolved) ORDER BY c.created_at DESC LIMIT 100`, v.OrgID, v.UserID, v.Admin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.EpisodeID, &c.Author.ID, &c.Author.Name, &c.Body, &c.Status, &c.CreatedAt, &c.Episode, &c.Reports); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- author stats ------------------------------------------------------------------

// EpisodeStats are the numbers an author sees for one episode.
type EpisodeStats struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	Plays          int     `json:"plays"`
	Listeners      int     `json:"listeners"`
	Completed      int     `json:"completed"`
	CompletionRate float64 `json:"completion_rate"`
	AvgPercent     float64 `json:"avg_percent"`
	Likes          int     `json:"likes"`
	Saves          int     `json:"saves"`
	Comments       int     `json:"comments"`
}

// DayCount is plays on one day.
type DayCount struct {
	Day   string `json:"day"`
	Plays int    `json:"plays"`
}

// ShowStats are the numbers for a show.
type ShowStats struct {
	Plays     int            `json:"plays"`
	Listeners int            `json:"listeners"`
	Followers int            `json:"followers"`
	Likes     int            `json:"likes"`
	Rate      float64        `json:"completion_rate"`
	Episodes  []EpisodeStats `json:"episodes"`
	Daily     []DayCount     `json:"daily"`
}

// Stats returns listening numbers for a show the viewer wrote.
func (s *Stories) Stats(ctx context.Context, v Viewer, showID string) (ShowStats, error) {
	var st ShowStats
	if err := s.authorOnly(ctx, v, showID); err != nil {
		return st, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT e.id::text, e.title, e.status,
		(SELECT count(*) FROM adrient.story_plays p WHERE p.episode_id=e.id),
		(SELECT count(*) FROM adrient.story_progress g WHERE g.episode_id=e.id),
		(SELECT count(*) FROM adrient.story_progress g WHERE g.episode_id=e.id AND g.completed),
		COALESCE((SELECT avg(LEAST(1.0, g.position_ms::float / NULLIF(g.duration_ms,0))) FROM adrient.story_progress g WHERE g.episode_id=e.id AND g.duration_ms>0),0),
		(SELECT count(*) FROM adrient.story_likes l WHERE l.episode_id=e.id),
		(SELECT count(*) FROM adrient.story_library b WHERE b.kind='episode' AND b.target_id=e.id),
		(SELECT count(*) FROM adrient.story_comments c WHERE c.episode_id=e.id AND c.status='visible')
		FROM adrient.story_episodes e WHERE e.show_id=$1::uuid ORDER BY e.position, e.created_at`, showID)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	st.Episodes = []EpisodeStats{}
	done := 0
	for rows.Next() {
		var e EpisodeStats
		var avg float64
		if err := rows.Scan(&e.ID, &e.Title, &e.Status, &e.Plays, &e.Listeners, &e.Completed, &avg, &e.Likes, &e.Saves, &e.Comments); err != nil {
			return st, err
		}
		if e.Listeners > 0 {
			e.CompletionRate = float64(e.Completed) / float64(e.Listeners)
		}
		e.AvgPercent = avg
		st.Plays += e.Plays
		st.Listeners += e.Listeners
		st.Likes += e.Likes
		done += e.Completed
		st.Episodes = append(st.Episodes, e)
	}
	if st.Listeners > 0 {
		st.Rate = float64(done) / float64(st.Listeners)
	}
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.story_follows WHERE kind='show' AND target_id=$1::uuid`, showID).Scan(&st.Followers)
	drows, err := s.Pool.Query(ctx, `SELECT to_char(d::date,'YYYY-MM-DD'), (SELECT count(*) FROM adrient.story_plays p JOIN adrient.story_episodes e ON e.id=p.episode_id
		WHERE e.show_id=$1::uuid AND p.created_at::date = d::date) FROM generate_series(current_date - 13, current_date, interval '1 day') d ORDER BY 1`, showID)
	if err == nil {
		for drows.Next() {
			var d DayCount
			if drows.Scan(&d.Day, &d.Plays) == nil {
				st.Daily = append(st.Daily, d)
			}
		}
		drows.Close()
	}
	return st, nil
}

// Audience is a group or department an author can limit a story to.
type AudienceOption struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Audiences lists the groups and departments of the org.
func (s *Stories) Audiences(ctx context.Context, org string) ([]AudienceOption, error) {
	out := []AudienceOption{}
	for _, q := range []struct{ kind, sql string }{
		{"group", `SELECT id::text, name FROM adrient.groups WHERE org_id=$1::uuid ORDER BY lower(name) LIMIT 300`},
		{"department", `SELECT id::text, name FROM adrient.departments WHERE org_id=$1::uuid ORDER BY lower(name) LIMIT 300`},
	} {
		rows, err := s.Pool.Query(ctx, q.sql, org)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			a := AudienceOption{Kind: q.kind}
			if rows.Scan(&a.ID, &a.Name) == nil {
				out = append(out, a)
			}
		}
		rows.Close()
	}
	return out, nil
}
