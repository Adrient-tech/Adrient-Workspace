// Package reposink implements cloudimport.Sink over the real Adrient
// repositories (Drive, Mail, Calendar, Contacts, Groups, Chat, sharing). It
// lives in its own package so internal/cloudimport stays free of app imports.
package reposink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/chat"
	"github.com/Adrient-tech/Adrient-Workspace/internal/cloudimport"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/groups"
	"github.com/Adrient-tech/Adrient-Workspace/internal/mail"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Blobs is the object-store slice the sink needs.
type Blobs interface {
	Put(ctx context.Context, key, mime string, size int64, r io.Reader) error
	Delete(ctx context.Context, key string) error
}

// Sink writes imported items. Any repository may be nil; that kind then fails
// with a clear error instead of being silently dropped.
type Sink struct {
	Pool     *pgxpool.Pool
	Drive    *drive.Repository
	Blobs    Blobs
	Sharing  *sharing.Repository
	Contacts *contacts.Repository
	Calendar *calendar.Repository
	Mail     *mail.Repository
	Groups   *groups.Repository
	Chat     *chat.Repository
}

var _ cloudimport.Sink = (*Sink)(nil)

func unavailable(app string) error { return fmt.Errorf("%s is not enabled on this server", app) }

func (s *Sink) FindUserByEmail(ctx context.Context, orgID, email string) (string, bool, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return "", false, nil
	}
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 AND (lower(email)=$2 OR lower(COALESCE(upn,''))=$2) ORDER BY created_at LIMIT 1`, orgID, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func (s *Sink) findChild(ctx context.Context, orgID, parent, name string, folder bool) (string, bool, error) {
	q := `SELECT id::text FROM adrient.drive_files WHERE org_id=$1 AND name=$2 AND trashed_at IS NULL AND (mime_type=$5) = $4 AND `
	args := []any{orgID, name, nil, folder, drive.FolderMimeType}
	if parent == "" {
		q += `parent_id IS NULL AND $3::uuid IS NULL`
	} else {
		q += `parent_id=$3`
		args[2] = parent
	}
	var id string
	err := s.Pool.QueryRow(ctx, q+` LIMIT 1`, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func (s *Sink) nameTaken(ctx context.Context, orgID, parent, name string) (bool, error) {
	q := `SELECT EXISTS(SELECT 1 FROM adrient.drive_files WHERE org_id=$1 AND name=$2 AND `
	args := []any{orgID, name}
	if parent == "" {
		q += `parent_id IS NULL)`
	} else {
		q += `parent_id=$3)`
		args = append(args, parent)
	}
	var ok bool
	err := s.Pool.QueryRow(ctx, q, args...).Scan(&ok)
	return ok, err
}

// freeName returns name, or "name (2).ext", "name (3).ext"... if taken.
func (s *Sink) freeName(ctx context.Context, orgID, parent, name string) (string, error) {
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i > 0 {
		base, ext = name[:i], name[i:]
	}
	cand := name
	for n := 2; n < 1000; n++ {
		taken, err := s.nameTaken(ctx, orgID, parent, cand)
		if err != nil {
			return "", err
		}
		if !taken {
			return cand, nil
		}
		cand = fmt.Sprintf("%s (%d)%s", base, n, ext)
	}
	return "", fmt.Errorf("too many files named %q", name)
}

func (s *Sink) EnsureFolder(ctx context.Context, orgID, ownerID, parent, name string) (string, error) {
	if s.Drive == nil {
		return "", unavailable("Drive")
	}
	if id, ok, err := s.findChild(ctx, orgID, parent, name, true); err != nil || ok {
		return id, err
	}
	n, err := s.freeName(ctx, orgID, parent, name) // a FILE of that name may exist
	if err != nil {
		return "", err
	}
	f, err := s.Drive.CreateFolder(ctx, orgID, ownerID, parent, n)
	if err != nil {
		return "", err
	}
	return f.ID, nil
}

func (s *Sink) PutFile(ctx context.Context, orgID, ownerID, parent, name, mime string, size int64, r io.Reader) (string, error) {
	if s.Drive == nil || s.Blobs == nil {
		return "", unavailable("Drive")
	}
	n, err := s.freeName(ctx, orgID, parent, name)
	if err != nil {
		return "", err
	}
	key, err := drive.NewStorageKey()
	if err != nil {
		return "", err
	}
	// Spool to disk: the vendor's size can be wrong (exports have none) and the
	// S3 client needs a known length or a seekable body over plain HTTP.
	tmp, err := os.CreateTemp("", "adrient-import-*")
	if err != nil {
		return "", err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	written, err := io.Copy(tmp, r)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if err := s.Blobs.Put(ctx, key, mime, written, tmp); err != nil {
		return "", err
	}
	f, err := s.Drive.CreateFile(ctx, orgID, ownerID, parent, n, mime, key, written)
	if err != nil {
		_ = s.Blobs.Delete(ctx, key)
		return "", err
	}
	return f.ID, nil
}

func (s *Sink) ShareFile(ctx context.Context, orgID, ownerID, fileID, grantee, role string) error {
	if s.Sharing == nil {
		return unavailable("sharing")
	}
	return s.Sharing.GrantAccess(ctx, "drive_file", fileID, grantee, role, ownerID)
}

func (s *Sink) AddContact(ctx context.Context, orgID, ownerID string, f cloudimport.ContactFields) (string, error) {
	if s.Contacts == nil {
		return "", unavailable("Contacts")
	}
	c, err := s.Contacts.Create(ctx, orgID, ownerID, contacts.Fields{DisplayName: f.DisplayName, FirstName: f.FirstName, LastName: f.LastName,
		Company: f.Company, JobTitle: f.JobTitle, Emails: f.Emails, Phones: f.Phones, Labels: f.Labels, Notes: f.Notes})
	return c.ID, err
}

func (s *Sink) AddEvent(ctx context.Context, orgID, ownerID string, f cloudimport.EventFields) (string, error) {
	if s.Calendar == nil {
		return "", unavailable("Calendar")
	}
	if f.StartAt.IsZero() {
		return "", errors.New("event has no start time")
	}
	if f.EndAt.Before(f.StartAt) {
		f.EndAt = f.StartAt
	}
	ev, err := s.Calendar.Create(ctx, orgID, ownerID, calendar.Fields{Title: f.Title, Description: f.Description, Location: f.Location,
		StartAt: f.StartAt, EndAt: f.EndAt, AllDay: f.AllDay, Recurrence: f.Recurrence})
	return ev.ID, err
}

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 140 {
		return string(r[:140])
	}
	return s
}

func (s *Sink) AddMail(ctx context.Context, orgID, ownerID string, m cloudimport.MailFields) (string, error) {
	if s.Mail == nil {
		return "", unavailable("Mail")
	}
	at := m.SentAt
	if at.IsZero() {
		at = time.Now()
	}
	out, err := s.Mail.Insert(ctx, mail.Message{OrgID: orgID, OwnerID: ownerID, Folder: m.Folder, FromAddr: m.FromAddr, FromName: m.FromName,
		ToAddrs: m.To, CcAddrs: m.Cc, Subject: m.Subject, Body: m.Body, Snippet: snippet(m.Body), IsRead: m.Read, Starred: m.Starred,
		Labels: m.Labels, SentAt: at})
	return out.ID, err
}

func (s *Sink) LabelMail(ctx context.Context, orgID, ownerID, mailID string, labels []string) error {
	if s.Mail == nil {
		return unavailable("Mail")
	}
	cur, err := s.Mail.Get(ctx, ownerID, mailID)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, l := range cur.Labels {
		have[l] = true
	}
	changed := false
	for _, l := range labels {
		if !have[l] {
			cur.Labels = append(cur.Labels, l)
			have[l], changed = true, true
		}
	}
	if !changed {
		return nil
	}
	_, err = s.Mail.Modify(ctx, ownerID, mailID, mail.Changes{IsRead: cur.IsRead, Starred: cur.Starred, Folder: cur.Folder, Labels: cur.Labels, SetLabels: true})
	return err
}

func (s *Sink) AddChat(ctx context.Context, orgID, ownerID string, c cloudimport.RemoteChat) (string, error) {
	if s.Chat == nil {
		return "", unavailable("Chat")
	}
	name := "Imported: " + c.Space
	var chID string
	err := s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.chat_channels WHERE org_id=$1 AND kind='group' AND name=$2 AND member_ids @> to_jsonb($3::text) LIMIT 1`,
		orgID, name, ownerID).Scan(&chID)
	if errors.Is(err, pgx.ErrNoRows) {
		ch, err := s.Chat.CreateChannel(ctx, orgID, "group", name, []string{ownerID})
		if err != nil {
			return "", err
		}
		chID = ch.ID
	} else if err != nil {
		return "", err
	}
	// The chat store stamps messages with the import time, so the original
	// time and author are kept in the text.
	body := fmt.Sprintf("[%s · %s] %s", c.At.UTC().Format("2006-01-02 15:04 UTC"), c.Author, c.Text)
	msg, err := s.Chat.PostMessage(ctx, chID, orgID, ownerID, c.Author, body, "")
	return msg.ID, err
}

func (s *Sink) EnsureGroup(ctx context.Context, orgID, ownerID, name, email string, members []string) (string, error) {
	if s.Groups == nil {
		return "", unavailable("Groups")
	}
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.groups WHERE org_id=$1 AND (lower(name)=lower($2) OR ($3<>'' AND lower(email)=lower($3))) LIMIT 1`, orgID, name, email).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	g, err := s.Groups.Create(ctx, orgID, ownerID, groups.GroupFields{Name: name, Email: email, Description: "Imported", MemberIDs: members})
	return g.ID, err
}
