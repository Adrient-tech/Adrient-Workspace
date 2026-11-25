package keep

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Limits for attachments.
const (
	MaxAttachmentBytes    = 2 << 20
	MaxAttachmentsPerNote = 12
)

// ErrInvalid marks bad input.
var ErrInvalid = errors.New("invalid input")

// Notebook groups notes (a section).
type Notebook struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AttachmentRef points at a stored image or drawing.
type AttachmentRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// Index is everything the Keep page needs beside the notes themselves.
type Index struct {
	Notebooks    []Notebook                 `json:"notebooks"`
	NoteNotebook map[string]string          `json:"note_notebook"`
	Attachments  map[string][]AttachmentRef `json:"attachments"`
}

// SniffImage returns the canonical MIME type for allowed image bytes, or "".
// SVG is deliberately not allowed (it can carry script).
func SniffImage(b []byte) string {
	switch ct := http.DetectContentType(b); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return ct
	}
	return ""
}

// BuildIndex loads notebooks, filing and attachments for the caller's own notes
// and for notes shared with them.
func (r *Repository) BuildIndex(ctx context.Context, org, user string) (Index, error) {
	idx := Index{Notebooks: []Notebook{}, NoteNotebook: map[string]string{}, Attachments: map[string][]AttachmentRef{}}
	rows, err := r.pool.Query(ctx, `SELECT id::text, name FROM adrient.keep_notebooks WHERE org_id=$1 AND owner_id=$2 ORDER BY position, created_at`, org, user)
	if err != nil {
		return idx, err
	}
	for rows.Next() {
		var n Notebook
		if err := rows.Scan(&n.ID, &n.Name); err != nil {
			rows.Close()
			return idx, err
		}
		idx.Notebooks = append(idx.Notebooks, n)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT m.note_id::text, m.notebook_id::text FROM adrient.keep_note_notebook m
		JOIN adrient.keep_notebooks b ON b.id=m.notebook_id WHERE b.org_id=$1 AND b.owner_id=$2`, org, user)
	if err != nil {
		return idx, err
	}
	for rows.Next() {
		var n, b string
		if err := rows.Scan(&n, &b); err != nil {
			rows.Close()
			return idx, err
		}
		idx.NoteNotebook[n] = b
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT a.id::text, a.note_id::text, a.kind FROM adrient.keep_attachments a
		JOIN adrient.keep_notes n ON n.id=a.note_id
		WHERE n.trashed_at IS NULL AND (n.owner_id=$2
		   OR EXISTS (SELECT 1 FROM adrient.object_grants g WHERE g.object_id=a.note_id AND g.grantee_user_id=$2 AND g.object_type='keep_note'))
		  AND a.org_id=$1 ORDER BY a.created_at`, org, user)
	if err != nil {
		// object_grants layout may differ on trimmed installs: fall back to own notes.
		rows, err = r.pool.Query(ctx, `SELECT a.id::text, a.note_id::text, a.kind FROM adrient.keep_attachments a
			JOIN adrient.keep_notes n ON n.id=a.note_id WHERE n.trashed_at IS NULL AND n.owner_id=$2 AND a.org_id=$1 ORDER BY a.created_at`, org, user)
		if err != nil {
			return idx, err
		}
	}
	for rows.Next() {
		var id, note, kind string
		if err := rows.Scan(&id, &note, &kind); err != nil {
			rows.Close()
			return idx, err
		}
		idx.Attachments[note] = append(idx.Attachments[note], AttachmentRef{ID: id, Kind: kind})
	}
	rows.Close()
	return idx, nil
}

// CreateNotebook adds a notebook.
func (r *Repository) CreateNotebook(ctx context.Context, org, user, name string) (Notebook, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return Notebook{}, fmt.Errorf("%w: a name of 1 to 80 characters is required", ErrInvalid)
	}
	var n Notebook
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.keep_notebooks (org_id, owner_id, name, position)
		VALUES ($1,$2,$3,COALESCE((SELECT MAX(position)+1 FROM adrient.keep_notebooks WHERE org_id=$1 AND owner_id=$2),0))
		RETURNING id::text, name`, org, user, name).Scan(&n.ID, &n.Name)
	return n, err
}

// RenameNotebook renames one of the user's notebooks.
func (r *Repository) RenameNotebook(ctx context.Context, org, user, id, name string) (Notebook, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 {
		return Notebook{}, fmt.Errorf("%w: a name of 1 to 80 characters is required", ErrInvalid)
	}
	var n Notebook
	err := r.pool.QueryRow(ctx, `UPDATE adrient.keep_notebooks SET name=$4 WHERE id=$1 AND org_id=$2 AND owner_id=$3 RETURNING id::text, name`, id, org, user, name).Scan(&n.ID, &n.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

// DeleteNotebook removes a notebook; its notes become unfiled (they are not deleted).
func (r *Repository) DeleteNotebook(ctx context.Context, org, user, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.keep_notebooks WHERE id=$1 AND org_id=$2 AND owner_id=$3`, id, org, user)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveNote files a note the user owns into a notebook ("" unfiles it).
func (r *Repository) MoveNote(ctx context.Context, org, user, noteID, notebookID string) error {
	var owner string
	if err := r.pool.QueryRow(ctx, `SELECT owner_id::text FROM adrient.keep_notes WHERE id=$1 AND org_id=$2 AND trashed_at IS NULL`, noteID, org).Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if owner != user {
		return fmt.Errorf("%w: only the owner can file a note", ErrInvalid)
	}
	if notebookID == "" {
		_, err := r.pool.Exec(ctx, `DELETE FROM adrient.keep_note_notebook WHERE note_id=$1`, noteID)
		return err
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.keep_notebooks WHERE id=$1 AND org_id=$2 AND owner_id=$3`, notebookID, org, user).Scan(&n); err != nil || n == 0 {
		return ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.keep_note_notebook (note_id, notebook_id) VALUES ($1,$2)
		ON CONFLICT (note_id) DO UPDATE SET notebook_id=$2`, noteID, notebookID)
	return err
}

// AddAttachment stores an image or drawing on a note.
func (r *Repository) AddAttachment(ctx context.Context, org, noteID, kind string, data []byte) (AttachmentRef, error) {
	if kind != "image" && kind != "drawing" {
		return AttachmentRef{}, fmt.Errorf("%w: kind must be image or drawing", ErrInvalid)
	}
	if len(data) == 0 || len(data) > MaxAttachmentBytes {
		return AttachmentRef{}, fmt.Errorf("%w: the file must be between 1 byte and 2 MB", ErrInvalid)
	}
	mime := SniffImage(data)
	if mime == "" {
		return AttachmentRef{}, fmt.Errorf("%w: only PNG, JPEG, GIF and WebP images are allowed", ErrInvalid)
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.keep_attachments WHERE note_id=$1`, noteID).Scan(&n); err != nil {
		return AttachmentRef{}, err
	}
	if n >= MaxAttachmentsPerNote {
		return AttachmentRef{}, fmt.Errorf("%w: at most %d images per note", ErrInvalid, MaxAttachmentsPerNote)
	}
	ref := AttachmentRef{Kind: kind}
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.keep_attachments (note_id, org_id, kind, mime, data, size_bytes)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`, noteID, org, kind, mime, data, len(data)).Scan(&ref.ID)
	return ref, err
}

// GetAttachment returns the bytes of an attachment together with its note id.
func (r *Repository) GetAttachment(ctx context.Context, org, id string) (noteID, mime string, data []byte, err error) {
	err = r.pool.QueryRow(ctx, `SELECT note_id::text, mime, data FROM adrient.keep_attachments WHERE id=$1 AND org_id=$2`, id, org).Scan(&noteID, &mime, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

// DeleteAttachment removes an attachment.
func (r *Repository) DeleteAttachment(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.keep_attachments WHERE id=$1 AND org_id=$2`, id, org)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Notifier delivers a notification.
type Notifier func(ctx context.Context, org, user, title, body, target string) error

type dueNote struct {
	ID, Org, Owner, Title, Body string
	At                          time.Time
}

// notesDue picks reminders that fired within the lookback window.
func notesDue(rows []dueNote, now time.Time, lookback time.Duration) []dueNote {
	var out []dueNote
	for _, n := range rows {
		if !n.At.After(now) && n.At.After(now.Add(-lookback)) {
			out = append(out, n)
		}
	}
	return out
}

// RunReminders notifies owners of notes whose reminder time has come. Each
// reminder is recorded first, so it is never delivered twice.
func (r *Repository) RunReminders(ctx context.Context, now time.Time, notify Notifier) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, org_id::text, owner_id::text, title, body, remind_at FROM adrient.keep_notes
		WHERE trashed_at IS NULL AND remind_at IS NOT NULL AND remind_at <= $1 AND remind_at > $2 LIMIT 2000`, now, now.Add(-time.Hour))
	if err != nil {
		return 0, err
	}
	var all []dueNote
	for rows.Next() {
		var n dueNote
		if err := rows.Scan(&n.ID, &n.Org, &n.Owner, &n.Title, &n.Body, &n.At); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, n)
	}
	rows.Close()
	sent := 0
	for _, n := range notesDue(all, now, 30*time.Minute) {
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.keep_reminders_sent (note_id, remind_at) VALUES ($1,$2) ON CONFLICT DO NOTHING`, n.ID, n.At)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		title := n.Title
		if strings.TrimSpace(title) == "" {
			title = firstLine(n.Body)
		}
		if title == "" {
			title = "Note reminder"
		}
		if notify != nil && notify(ctx, n.Org, n.Owner, "Reminder: "+title, "From Keep", "/keep") == nil {
			sent++
		}
	}
	return sent, nil
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			if utf8.RuneCountInString(t) > 80 {
				return string([]rune(t)[:80])
			}
			return t
		}
	}
	return ""
}
