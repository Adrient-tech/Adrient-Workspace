package music

import (
	"bytes"
	"context"
	"encoding/base64"
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

// Extras serves the Music app's JSON API under /api/v1/music/x/: the library
// view with play counts and likes, play history, smart playlists, lyrics,
// playlist visibility and ordering, and import from Drive. The gRPC service
// keeps serving the basic CRUD; nothing here changes its contract.
type Extras struct {
	pool  *pgxpool.Pool
	blobs BlobStore
}

// NewExtras constructs the handler.
func NewExtras(pool *pgxpool.Pool, blobs BlobStore) *Extras {
	return &Extras{pool: pool, blobs: blobs}
}

const extrasMount = "/api/v1/music/x/"

// Match reports whether the path belongs to the extras API.
func (e *Extras) Match(p string) bool { return strings.HasPrefix(p, extrasMount) }

// LibTrack is a track as the web app sees it.
type LibTrack struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"owner_id"`
	Title       string     `json:"title"`
	Artist      string     `json:"artist"`
	Album       string     `json:"album"`
	Genre       string     `json:"genre"`
	TrackNo     int        `json:"track_no"`
	Year        int        `json:"year"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	Duration    float64    `json:"duration_seconds"`
	StreamURL   string     `json:"stream_url"`
	ArtURL      string     `json:"art_url"`
	HasLyrics   bool       `json:"has_lyrics"`
	Liked       bool       `json:"liked"`
	PlayCount   int        `json:"play_count"`
	LastPlayed  *time.Time `json:"last_played,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

const libSelect = `SELECT t.id::text, t.owner_id::text, t.title, t.artist, t.album, t.genre, t.track_no, t.year,
	t.content_type, t.size, t.duration_seconds, (t.artwork_data_url <> ''), (t.lyrics <> ''),
	EXISTS(SELECT 1 FROM adrient.music_likes l WHERE l.track_id=t.id AND l.user_id=$2::uuid),
	(SELECT count(*) FROM adrient.music_plays p WHERE p.track_id=t.id AND p.user_id=$2::uuid),
	(SELECT max(p.played_at) FROM adrient.music_plays p WHERE p.track_id=t.id AND p.user_id=$2::uuid),
	t.created_at, t.updated_at
	FROM adrient.music_tracks t`

func scanLib(rows pgx.Rows) ([]LibTrack, error) {
	defer rows.Close()
	out := []LibTrack{}
	for rows.Next() {
		var t LibTrack
		var hasArt bool
		var upd time.Time
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.Title, &t.Artist, &t.Album, &t.Genre, &t.TrackNo, &t.Year,
			&t.ContentType, &t.Size, &t.Duration, &hasArt, &t.HasLyrics, &t.Liked, &t.PlayCount, &t.LastPlayed,
			&t.CreatedAt, &upd); err != nil {
			return nil, err
		}
		t.StreamURL = "/api/v1/music/" + t.ID + "/content"
		if hasArt {
			t.ArtURL = extrasMount + "art/" + t.ID + "?v=" + strconv.FormatInt(upd.Unix(), 10)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Library returns every non-trashed track of the org with the caller's stats.
func (e *Extras) Library(ctx context.Context, orgID, userID string) ([]LibTrack, error) {
	rows, err := e.pool.Query(ctx, libSelect+` WHERE t.org_id=$1::uuid AND t.trashed_at IS NULL ORDER BY t.created_at DESC`, orgID, userID)
	if err != nil {
		return nil, err
	}
	return scanLib(rows)
}

// Smart returns a computed list: most_played, recently_played, recently_added, liked.
func (e *Extras) Smart(ctx context.Context, orgID, userID, kind string, limit int) ([]LibTrack, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var tail string
	switch kind {
	case "most_played":
		tail = ` AND EXISTS(SELECT 1 FROM adrient.music_plays p WHERE p.track_id=t.id AND p.user_id=$2::uuid)
			ORDER BY (SELECT count(*) FROM adrient.music_plays p WHERE p.track_id=t.id AND p.user_id=$2::uuid) DESC, t.title`
	case "recently_played":
		tail = ` AND EXISTS(SELECT 1 FROM adrient.music_plays p WHERE p.track_id=t.id AND p.user_id=$2::uuid)
			ORDER BY (SELECT max(p.played_at) FROM adrient.music_plays p WHERE p.track_id=t.id AND p.user_id=$2::uuid) DESC`
	case "recently_added":
		tail = ` ORDER BY t.created_at DESC`
	case "liked":
		tail = ` AND EXISTS(SELECT 1 FROM adrient.music_likes l WHERE l.track_id=t.id AND l.user_id=$2::uuid)
			ORDER BY (SELECT l.liked_at FROM adrient.music_likes l WHERE l.track_id=t.id AND l.user_id=$2::uuid) DESC`
	default:
		return nil, errors.New("unknown smart list")
	}
	rows, err := e.pool.Query(ctx, libSelect+` WHERE t.org_id=$1::uuid AND t.trashed_at IS NULL`+tail+fmt.Sprintf(" LIMIT %d", limit), orgID, userID)
	if err != nil {
		return nil, err
	}
	return scanLib(rows)
}

// RecordPlay logs one listen of a track by the user.
func (e *Extras) RecordPlay(ctx context.Context, orgID, userID, trackID string, seconds float64) error {
	if seconds < 0 || seconds > 86400 {
		seconds = 0
	}
	tag, err := e.pool.Exec(ctx, `INSERT INTO adrient.music_plays (org_id, user_id, track_id, seconds_listened)
		SELECT $1::uuid, $2::uuid, t.id, $4 FROM adrient.music_tracks t
		WHERE t.id=$3::uuid AND t.org_id=$1::uuid AND t.trashed_at IS NULL`, orgID, userID, trackID, seconds)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// History returns the user's most recent plays, newest first (one row per play).
func (e *Extras) History(ctx context.Context, orgID, userID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := e.pool.Query(ctx, `SELECT p.id::text, p.played_at, p.seconds_listened, t.id::text, t.title, t.artist, t.album, t.duration_seconds,
		(t.artwork_data_url <> ''), t.updated_at
		FROM adrient.music_plays p JOIN adrient.music_tracks t ON t.id=p.track_id
		WHERE p.org_id=$1::uuid AND p.user_id=$2::uuid AND t.trashed_at IS NULL
		ORDER BY p.played_at DESC LIMIT $3`, orgID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var pid, tid, title, artist, album string
		var at, upd time.Time
		var secs, dur float64
		var art bool
		if err := rows.Scan(&pid, &at, &secs, &tid, &title, &artist, &album, &dur, &art, &upd); err != nil {
			return nil, err
		}
		artURL := ""
		if art {
			artURL = extrasMount + "art/" + tid + "?v=" + strconv.FormatInt(upd.Unix(), 10)
		}
		out = append(out, map[string]any{"play_id": pid, "played_at": at, "seconds_listened": secs, "track_id": tid,
			"title": title, "artist": artist, "album": album, "duration_seconds": dur, "art_url": artURL,
			"stream_url": "/api/v1/music/" + tid + "/content"})
	}
	return out, rows.Err()
}

// Lyrics returns the stored lyrics (plain text or .lrc timed lines).
func (e *Extras) Lyrics(ctx context.Context, orgID, trackID string) (string, error) {
	var l string
	err := e.pool.QueryRow(ctx, `SELECT lyrics FROM adrient.music_tracks WHERE id=$1::uuid AND org_id=$2::uuid AND trashed_at IS NULL`, trackID, orgID).Scan(&l)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return l, err
}

// SetLyrics stores lyrics for a track the user owns.
func (e *Extras) SetLyrics(ctx context.Context, orgID, userID, trackID, lyrics string) error {
	if len(lyrics) > 200_000 {
		return errors.New("lyrics too large")
	}
	tag, err := e.pool.Exec(ctx, `UPDATE adrient.music_tracks SET lyrics=$4, updated_at=now()
		WHERE id=$1::uuid AND org_id=$2::uuid AND owner_id=$3::uuid AND trashed_at IS NULL`, trackID, orgID, userID, lyrics)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ArtworkBytes decodes a track's cover data URL.
func (e *Extras) ArtworkBytes(ctx context.Context, orgID, trackID string) ([]byte, string, time.Time, error) {
	var du string
	var upd time.Time
	err := e.pool.QueryRow(ctx, `SELECT artwork_data_url, updated_at FROM adrient.music_tracks
		WHERE id=$1::uuid AND org_id=$2::uuid AND trashed_at IS NULL`, trackID, orgID).Scan(&du, &upd)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && du == "") {
		return nil, "", time.Time{}, ErrNotFound
	}
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return DecodeDataURL(du, upd)
}

// DecodeDataURL splits "data:<mime>;base64,<payload>" into bytes and mime.
func DecodeDataURL(du string, upd time.Time) ([]byte, string, time.Time, error) {
	if !strings.HasPrefix(du, "data:") {
		return nil, "", upd, ErrNotFound
	}
	comma := strings.IndexByte(du, ',')
	if comma < 0 {
		return nil, "", upd, ErrNotFound
	}
	meta := du[5:comma]
	mime := strings.TrimSuffix(meta, ";base64")
	if !strings.HasPrefix(mime, "image/") || mime == "image/svg+xml" {
		return nil, "", upd, ErrNotFound
	}
	b, err := base64.StdEncoding.DecodeString(du[comma+1:])
	if err != nil {
		return nil, "", upd, ErrNotFound
	}
	return b, mime, upd, nil
}

// readTagsFromBlob reads the ID3v2 header (and its body) from a blob.
func (e *Extras) readTagsFromBlob(ctx context.Context, key string) (Tags, error) {
	body, _, _, err := e.blobs.Get(ctx, key)
	if err != nil {
		return Tags{}, err
	}
	defer body.Close()
	return ReadTags(body), nil
}

// ReadTags parses an ID3v2 tag from the start of r (up to 8 MiB). A missing
// tag yields zero Tags.
func ReadTags(r io.Reader) Tags {
	head := make([]byte, 10)
	if _, err := io.ReadFull(r, head); err != nil || string(head[:3]) != "ID3" {
		return Tags{}
	}
	size := syncsafe(head[6:10])
	if size > 8<<20 {
		size = 8 << 20
	}
	buf := make([]byte, size)
	n, _ := io.ReadFull(r, buf)
	t, _ := ParseID3v2(append(head, buf[:n]...))
	return t
}

// ApplyTags fills in the metadata columns of a track from tags, only where the
// track's own value is still empty (so a title typed in the upload form wins).
func (e *Extras) ApplyTags(ctx context.Context, orgID, trackID string, t Tags, lyrics string) error {
	_, err := e.pool.Exec(ctx, `UPDATE adrient.music_tracks SET
		artist  = CASE WHEN artist='' THEN $3 ELSE artist END,
		album   = CASE WHEN album='' THEN $4 ELSE album END,
		genre   = CASE WHEN genre='' THEN $5 ELSE genre END,
		track_no= CASE WHEN track_no=0 THEN $6 ELSE track_no END,
		year    = CASE WHEN year=0 THEN $7 ELSE year END,
		lyrics  = CASE WHEN lyrics='' THEN $8 ELSE lyrics END,
		artwork_data_url = CASE WHEN artwork_data_url='' THEN $9 ELSE artwork_data_url END,
		updated_at = now()
		WHERE id=$1::uuid AND org_id=$2::uuid`, trackID, orgID, t.Artist, t.Album, t.Genre, t.TrackNo, t.Year, firstNonEmpty(lyrics, t.Lyrics), t.ArtDataURL())
	return err
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// Rescan re-reads the tags and cover art of an existing track from its file.
func (e *Extras) Rescan(ctx context.Context, orgID, trackID string) error {
	var key, title string
	err := e.pool.QueryRow(ctx, `SELECT blob_key, title FROM adrient.music_tracks WHERE id=$1::uuid AND org_id=$2::uuid AND trashed_at IS NULL`, trackID, orgID).Scan(&key, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	t, err := e.readTagsFromBlob(ctx, key)
	if err != nil {
		return err
	}
	if t.Title != "" && strings.TrimSpace(title) == "" {
		_, _ = e.pool.Exec(ctx, `UPDATE adrient.music_tracks SET title=$2 WHERE id=$1::uuid`, trackID, t.Title)
	}
	return e.ApplyTags(ctx, orgID, trackID, t, "")
}

// DriveAudio is one audio file of the Drive offered for import.
type DriveAudio struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Imported bool   `json:"imported"`
}

var audioExt = map[string]string{".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac", ".ogg": "audio/ogg",
	".opus": "audio/ogg", ".flac": "audio/flac", ".wav": "audio/wav"}

func isAudioFile(name, mime string) bool {
	if strings.HasPrefix(mime, "audio/") {
		return true
	}
	_, ok := audioExt[strings.ToLower(path.Ext(name))]
	return ok
}

// DriveAudioFiles lists audio files in the user's reachable Drive.
func (e *Extras) DriveAudioFiles(ctx context.Context, orgID, userID, q string) ([]DriveAudio, error) {
	rows, err := e.pool.Query(ctx, `SELECT f.id::text, f.name, f.mime_type, f.size_bytes,
		EXISTS(SELECT 1 FROM adrient.music_tracks t WHERE t.source_file_id=f.id AND t.trashed_at IS NULL)
		FROM adrient.drive_files f
		WHERE f.org_id=$1::uuid AND f.owner_id=$2::uuid AND f.trashed_at IS NULL AND f.storage_key IS NOT NULL
		  AND f.name ILIKE '%' || $3 || '%'
		ORDER BY f.updated_at DESC LIMIT 400`, orgID, userID, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DriveAudio{}
	for rows.Next() {
		var d DriveAudio
		var mime string
		if err := rows.Scan(&d.ID, &d.Name, &mime, &d.Size, &d.Imported); err != nil {
			return nil, err
		}
		if isAudioFile(d.Name, mime) {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

// ImportFromDrive adds a Drive audio file to the library without copying the
// bytes (the track points at the Drive blob). Tags, cover art and a sibling
// ".lrc" lyrics file in the same folder are picked up. Idempotent per file.
func (e *Extras) ImportFromDrive(ctx context.Context, orgID, userID, fileID string) (LibTrack, error) {
	var existing string
	err := e.pool.QueryRow(ctx, `SELECT id::text FROM adrient.music_tracks WHERE source_file_id=$1::uuid AND org_id=$2::uuid AND trashed_at IS NULL`, fileID, orgID).Scan(&existing)
	if err == nil {
		return e.one(ctx, orgID, userID, existing)
	}
	var name, mime string
	var key *string
	var size int64
	var parent *string
	err = e.pool.QueryRow(ctx, `SELECT name, mime_type, storage_key, size_bytes, parent_id::text FROM adrient.drive_files
		WHERE id=$1::uuid AND org_id=$2::uuid AND owner_id=$3::uuid AND trashed_at IS NULL`, fileID, orgID, userID).Scan(&name, &mime, &key, &size, &parent)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && key == nil) {
		return LibTrack{}, ErrNotFound
	}
	if err != nil {
		return LibTrack{}, err
	}
	if !isAudioFile(name, mime) {
		return LibTrack{}, errors.New("not an audio file")
	}
	ct := mime
	if !strings.HasPrefix(ct, "audio/") {
		ct = audioExt[strings.ToLower(path.Ext(name))]
	}
	tags, _ := e.readTagsFromBlob(ctx, *key)
	title := tags.Title
	if title == "" {
		title = trimExt(name)
	}
	var lyrics string
	if parent != nil {
		lrc := trimExt(name) + ".lrc"
		var lkey *string
		if e.pool.QueryRow(ctx, `SELECT storage_key FROM adrient.drive_files WHERE org_id=$1::uuid AND parent_id=$2::uuid AND lower(name)=lower($3) AND trashed_at IS NULL`,
			orgID, *parent, lrc).Scan(&lkey) == nil && lkey != nil {
			if b, _, _, err := e.blobs.Get(ctx, *lkey); err == nil {
				data, _ := io.ReadAll(io.LimitReader(b, 400_000))
				b.Close()
				lyrics = string(data)
			}
		}
	}
	dur := e.probeBlobDuration(ctx, *key, name)
	var id string
	err = e.pool.QueryRow(ctx, `INSERT INTO adrient.music_tracks (org_id, owner_id, title, artist, album, content_type, size, duration_seconds,
		artwork_data_url, blob_key, source_file_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::uuid) RETURNING id::text`,
		orgID, userID, title, tags.Artist, tags.Album, ct, size, dur, tags.ArtDataURL(), *key, fileID).Scan(&id)
	if err != nil {
		return LibTrack{}, err
	}
	_ = e.ApplyTags(ctx, orgID, id, tags, lyrics)
	return e.one(ctx, orgID, userID, id)
}

func (e *Extras) one(ctx context.Context, orgID, userID, id string) (LibTrack, error) {
	rows, err := e.pool.Query(ctx, libSelect+` WHERE t.id=$3::uuid AND t.org_id=$1::uuid AND t.trashed_at IS NULL`, orgID, userID, id)
	if err != nil {
		return LibTrack{}, err
	}
	l, err := scanLib(rows)
	if err != nil {
		return LibTrack{}, err
	}
	if len(l) == 0 {
		return LibTrack{}, ErrNotFound
	}
	return l[0], nil
}

// probeBlobDuration asks ffprobe (when installed) for a blob's duration; it
// returns 0 when ffprobe is missing or fails (the browser then fills it in).
func (e *Extras) probeBlobDuration(ctx context.Context, key, name string) float64 {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0
	}
	body, _, _, err := e.blobs.Get(ctx, key)
	if err != nil {
		return 0
	}
	defer body.Close()
	f, err := os.CreateTemp("", "mx-*"+path.Ext(name))
	if err != nil {
		return 0
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, io.LimitReader(body, 512<<20)); err != nil {
		f.Close()
		return 0
	}
	f.Close()
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", f.Name()).Output()
	if err != nil {
		return 0
	}
	d, _ := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	return d
}

// --- playlists --------------------------------------------------------------

// PlaylistInfo is a playlist as listed in the web app.
type PlaylistInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	OwnerName   string    `json:"owner_name"`
	Mine        bool      `json:"mine"`
	Visibility  string    `json:"visibility"`
	GroupID     string    `json:"group_id"`
	GroupName   string    `json:"group_name"`
	TrackCount  int       `json:"track_count"`
	CoverArt    []string  `json:"cover_art"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const visibleClause = `p.org_id=$1::uuid AND p.trashed_at IS NULL AND (
	p.owner_id=$2::uuid OR p.visibility='org'
	OR (p.visibility='group' AND p.group_id IS NOT NULL AND EXISTS(
		SELECT 1 FROM adrient.groups g WHERE g.id=p.group_id AND (g.owner_id=$2::uuid OR g.member_ids ? $2::text))))`

// Playlists lists the playlists the user may see.
func (e *Extras) Playlists(ctx context.Context, orgID, userID string) ([]PlaylistInfo, error) {
	rows, err := e.pool.Query(ctx, `SELECT p.id::text, p.name, p.description, p.owner_id::text, COALESCE(u.display_name,''),
		p.visibility, COALESCE(p.group_id::text,''), COALESCE(g.name,''), p.updated_at,
		(SELECT count(*) FROM adrient.music_playlist_tracks pt JOIN adrient.music_tracks t ON t.id=pt.track_id
		   WHERE pt.playlist_id=p.id AND t.trashed_at IS NULL),
		COALESCE((SELECT array_agg(x.id) FROM (SELECT t.id::text AS id FROM adrient.music_playlist_tracks pt
		   JOIN adrient.music_tracks t ON t.id=pt.track_id
		   WHERE pt.playlist_id=p.id AND t.trashed_at IS NULL AND t.artwork_data_url<>'' ORDER BY pt.position LIMIT 4) x), '{}')
		FROM adrient.music_playlists p
		LEFT JOIN adrient.users u ON u.id=p.owner_id
		LEFT JOIN adrient.groups g ON g.id=p.group_id
		WHERE `+visibleClause+` ORDER BY p.updated_at DESC`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlaylistInfo{}
	for rows.Next() {
		var p PlaylistInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.OwnerName, &p.Visibility, &p.GroupID, &p.GroupName,
			&p.UpdatedAt, &p.TrackCount, &p.CoverArt); err != nil {
			return nil, err
		}
		p.Mine = p.OwnerID == userID
		for i, id := range p.CoverArt {
			p.CoverArt[i] = extrasMount + "art/" + id
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Playlist returns one visible playlist and its ordered tracks.
func (e *Extras) Playlist(ctx context.Context, orgID, userID, id string) (PlaylistInfo, []LibTrack, error) {
	all, err := e.Playlists(ctx, orgID, userID)
	if err != nil {
		return PlaylistInfo{}, nil, err
	}
	for _, p := range all {
		if p.ID == id {
			rows, err := e.pool.Query(ctx, libSelect+` JOIN adrient.music_playlist_tracks pt ON pt.track_id=t.id
				WHERE pt.playlist_id=$3::uuid AND t.org_id=$1::uuid AND t.trashed_at IS NULL ORDER BY pt.position`, orgID, userID, id)
			if err != nil {
				return PlaylistInfo{}, nil, err
			}
			tr, err := scanLib(rows)
			return p, tr, err
		}
	}
	return PlaylistInfo{}, nil, ErrNotFound
}

// SetPlaylistSharing changes who can see a playlist the user owns.
func (e *Extras) SetPlaylistSharing(ctx context.Context, orgID, userID, id, visibility, groupID string) error {
	switch visibility {
	case "private", "org":
		groupID = ""
	case "group":
		if groupID == "" {
			return errors.New("group_id required")
		}
		var ok bool
		if err := e.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.groups g WHERE g.id=$1::uuid AND g.org_id=$2::uuid
			AND (g.owner_id=$3::uuid OR g.member_ids ? $3::text))`, groupID, orgID, userID).Scan(&ok); err != nil || !ok {
			return errors.New("not a member of that group")
		}
	default:
		return errors.New("visibility must be private, org or group")
	}
	var gid any
	if groupID != "" {
		gid = groupID
	}
	tag, err := e.pool.Exec(ctx, `UPDATE adrient.music_playlists SET visibility=$4, group_id=$5::uuid, updated_at=now()
		WHERE id=$1::uuid AND org_id=$2::uuid AND owner_id=$3::uuid AND trashed_at IS NULL`, id, orgID, userID, visibility, gid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderPlaylist sets the track order of a playlist the user owns. The list
// must contain exactly the playlist's current tracks.
func (e *Extras) ReorderPlaylist(ctx context.Context, orgID, userID, id string, trackIDs []string) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.music_playlists WHERE id=$1::uuid AND org_id=$2::uuid AND owner_id=$3::uuid AND trashed_at IS NULL`, id, orgID, userID).Scan(&n); err != nil || n == 0 {
		return ErrNotFound
	}
	var have int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM adrient.music_playlist_tracks WHERE playlist_id=$1::uuid`, id).Scan(&have)
	if have != len(trackIDs) {
		return errors.New("track list does not match the playlist")
	}
	for i, tid := range trackIDs {
		tag, err := tx.Exec(ctx, `UPDATE adrient.music_playlist_tracks SET position=$3 WHERE playlist_id=$1::uuid AND track_id=$2::uuid`, id, tid, i)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("track not in playlist")
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.music_playlists SET updated_at=now() WHERE id=$1::uuid`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Groups lists the groups the user belongs to (for sharing a playlist).
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

// --- HTTP -------------------------------------------------------------------

func xJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func xFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		msg := err.Error()
		if strings.Contains(msg, "required") || strings.Contains(msg, "must be") || strings.Contains(msg, "not a ") || strings.Contains(msg, "does not match") || strings.Contains(msg, "not in playlist") || strings.Contains(msg, "too large") || strings.Contains(msg, "unknown") {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		http.Error(w, "server error", http.StatusInternalServerError)
	}
}

func xDecode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// ServeHTTP routes /api/v1/music/x/*.
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
	seg := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, extrasMount), "/"), "/")
	m := r.Method
	switch {
	case m == http.MethodGet && len(seg) == 1 && seg[0] == "library":
		l, err := e.Library(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"tracks": l})
	case m == http.MethodGet && len(seg) == 2 && seg[0] == "smart":
		lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		l, err := e.Smart(ctx, o.ID, u.ID, seg[1], lim)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"tracks": l})
	case m == http.MethodGet && len(seg) == 2 && seg[0] == "art":
		b, mime, upd, err := e.ArtworkBytes(ctx, o.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeContent(w, r, "", upd, bytes.NewReader(b))
	case m == http.MethodPost && len(seg) == 1 && seg[0] == "plays":
		var in struct {
			TrackID string  `json:"track_id"`
			Seconds float64 `json:"seconds"`
		}
		if xDecode(r, &in) != nil || in.TrackID == "" {
			http.Error(w, "track_id required", http.StatusBadRequest)
			return
		}
		if err := e.RecordPlay(ctx, o.ID, u.ID, in.TrackID, in.Seconds); err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]bool{"ok": true})
	case m == http.MethodGet && len(seg) == 1 && seg[0] == "history":
		lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		h, err := e.History(ctx, o.ID, u.ID, lim)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"plays": h})
	case len(seg) == 3 && seg[0] == "tracks" && seg[2] == "lyrics" && m == http.MethodGet:
		l, err := e.Lyrics(ctx, o.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"lyrics": l, "timed": IsLRC(l)})
	case len(seg) == 3 && seg[0] == "tracks" && seg[2] == "lyrics" && m == http.MethodPut:
		var in struct {
			Lyrics string `json:"lyrics"`
		}
		if xDecode(r, &in) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := e.SetLyrics(ctx, o.ID, u.ID, seg[1], in.Lyrics); err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"timed": IsLRC(in.Lyrics)})
	case len(seg) == 3 && seg[0] == "tracks" && seg[2] == "rescan" && m == http.MethodPost:
		if err := e.Rescan(ctx, o.ID, seg[1]); err != nil {
			xFail(w, err)
			return
		}
		t, err := e.one(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, t)
	case m == http.MethodGet && len(seg) == 1 && seg[0] == "drive-files":
		l, err := e.DriveAudioFiles(ctx, o.ID, u.ID, r.URL.Query().Get("q"))
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"files": l})
	case m == http.MethodPost && len(seg) == 1 && seg[0] == "import-drive":
		var in struct {
			FileID string `json:"file_id"`
		}
		if xDecode(r, &in) != nil || in.FileID == "" {
			http.Error(w, "file_id required", http.StatusBadRequest)
			return
		}
		t, err := e.ImportFromDrive(ctx, o.ID, u.ID, in.FileID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, t)
	case m == http.MethodGet && len(seg) == 1 && seg[0] == "groups":
		g, err := e.Groups(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"groups": g})
	case m == http.MethodGet && len(seg) == 1 && seg[0] == "playlists":
		p, err := e.Playlists(ctx, o.ID, u.ID)
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"playlists": p})
	case m == http.MethodGet && len(seg) == 2 && seg[0] == "playlists":
		p, tr, err := e.Playlist(ctx, o.ID, u.ID, seg[1])
		if err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]any{"playlist": p, "tracks": tr})
	case m == http.MethodPatch && len(seg) == 2 && seg[0] == "playlists":
		var in struct {
			Visibility string `json:"visibility"`
			GroupID    string `json:"group_id"`
		}
		if xDecode(r, &in) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := e.SetPlaylistSharing(ctx, o.ID, u.ID, seg[1], in.Visibility, in.GroupID); err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]bool{"ok": true})
	case m == http.MethodPut && len(seg) == 3 && seg[0] == "playlists" && seg[2] == "order":
		var in struct {
			TrackIDs []string `json:"track_ids"`
		}
		if xDecode(r, &in) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if err := e.ReorderPlaylist(ctx, o.ID, u.ID, seg[1], in.TrackIDs); err != nil {
			xFail(w, err)
			return
		}
		xJSON(w, 200, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// filterVisible drops playlists the user may not see (private ones of other
// users, group ones for non-members). On a lookup error it fails closed for
// other people's playlists.
func (r *Repository) filterVisible(ctx context.Context, orgID, userID string, in []Playlist) []Playlist {
	if len(in) == 0 {
		return in
	}
	ids := make([]string, len(in))
	for i, p := range in {
		ids[i] = p.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT p.id::text FROM adrient.music_playlists p WHERE p.id = ANY($3::uuid[]) AND `+visibleClause, orgID, userID, ids)
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
	for _, p := range in {
		if allowed[p.ID] || (err != nil && p.OwnerID == userID) {
			out = append(out, p)
		}
	}
	return out
}
