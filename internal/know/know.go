// Package know wires the knowledge modules (records, wiki, contracts,
// signatures, legal, captable) to each other and to Drive, Calendar, Tasks,
// Contacts and mail, and builds their HTTP handlers.
package know

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/captable"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contracts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/legal"
	"github.com/Adrient-tech/Adrient-Workspace/internal/notifications"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opsports"
	"github.com/Adrient-tech/Adrient-Workspace/internal/records"
	"github.com/Adrient-tech/Adrient-Workspace/internal/signatures"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tasks"
	"github.com/Adrient-tech/Adrient-Workspace/internal/wiki"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config carries the other modules the knowledge modules use. Every field is optional.
type Config struct {
	Pool  *pgxpool.Pool
	Blobs *drive.Blobs
	Drive *drive.Repository
	// Calendar, Tasks and Contacts connect deadlines, reminders and parties.
	Calendar *calendar.Repository
	Tasks    *tasks.Repository
	Contacts *contacts.Repository
	// Mail sends transactional mail (signing links); Notifications feeds the bell.
	Mail          *email.Sender
	Notifications *notifications.Repository
	// Invoicing receives the draft invoices Legal creates from bills.
	Invoicing *invoicing.Repository
}

// Set holds the repositories.
type Set struct {
	pool       *pgxpool.Pool
	Records    *records.Repository
	Wiki       *wiki.Repository
	Contracts  *contracts.Repository
	Signatures *signatures.Repository
	Legal      *legal.Repository
	CapTable   *captable.Repository
	publicH    []knowkit.Public
	files      *driveFiles
	handler    []*knowkit.Handler
}

// New builds the repositories and connects them.
func New(c Config) *Set {
	s := &Set{pool: c.Pool, Records: records.NewRepository(c.Pool), Wiki: wiki.NewRepository(c.Pool), Contracts: contracts.NewRepository(c.Pool), Signatures: signatures.NewRepository(c.Pool),
		Legal: legal.NewRepository(c.Pool), CapTable: captable.NewRepository(c.Pool)}
	s.wireContracts(c)
	s.wireLegal(c)
	s.wireSignatures(c)
	if c.Blobs != nil && c.Drive != nil {
		s.files = &driveFiles{blobs: c.Blobs, drive: c.Drive}
	}
	s.Wiki.Deps.Records = s.Records
	s.Records.Deps.ExternalRole = func(ctx context.Context, org, user, source, ref string) int {
		if source == "wiki" {
			return s.Wiki.SpaceRole(ctx, org, user, false, ref)
		}
		return 0
	}
	if s.files != nil {
		s.Records.Deps.FileMeta = s.files.meta
		s.Records.Deps.SaveFile = s.files.save
	}
	return s
}

// Handlers returns the authenticated HTTP handlers. audit receives (app, action, details).
func (s *Set) Handlers(auth knowkit.Auth, audit func(ctx context.Context, app, action string, d map[string]any)) []*knowkit.Handler {
	hook := func(app string) knowkit.Audit {
		return func(ctx context.Context, action, typ, id string) {
			if audit != nil {
				audit(ctx, app, action, map[string]any{"type": typ, "id": id})
			}
		}
	}
	s.Records.Deps.Audit = hook("records")
	s.Wiki.Deps.Audit = hook("wiki")
	s.Contracts.Deps.Audit = hook("contracts")
	s.Signatures.Deps.Audit = hook("signatures")
	s.Legal.Deps.Audit = hook("legal")
	s.CapTable.Deps.Audit = hook("captable")
	return []*knowkit.Handler{
		records.NewHTTPHandler(s.Records, auth),
		wiki.NewHTTPHandler(s.Wiki, auth),
		contracts.NewHTTPHandler(s.Contracts, auth),
		signatures.NewHTTPHandler(s.Signatures, auth),
		legal.NewHTTPHandler(s.Legal, auth),
		captable.NewHTTPHandler(s.CapTable, auth),
	}
}

// driveFiles stores and reads files in Drive for the knowledge modules.
type driveFiles struct {
	blobs *drive.Blobs
	drive *drive.Repository
}

// meta checks that the user may attach a Drive file (their own, not trashed).
func (d *driveFiles) meta(ctx context.Context, org, user, fileID string) (string, string, int64, error) {
	f, err := d.drive.Get(ctx, org, fileID)
	if err != nil || f.TrashedAt != nil || f.MimeType == drive.FolderMimeType {
		return "", "", 0, fmt.Errorf("file not found")
	}
	if f.OwnerID != user {
		return "", "", 0, fmt.Errorf("you can only attach your own files")
	}
	return f.Name, f.MimeType, f.SizeBytes, nil
}

// save writes bytes as a new Drive file in the user's root.
func (d *driveFiles) save(ctx context.Context, org, user, name, mime string, data []byte) (string, error) {
	key := fmt.Sprintf("blobs/know/%s-%d", knowkit.NewKey(), time.Now().UnixNano())
	if err := d.blobs.Put(ctx, key, mime, int64(len(data)), bytes.NewReader(data)); err != nil {
		return "", err
	}
	f, err := d.drive.CreateFile(ctx, org, user, "", name, mime, key, int64(len(data)))
	if err != nil {
		_ = d.blobs.Delete(ctx, key)
		return "", err
	}
	return f.ID, nil
}

// read returns a Drive file's bytes (any file of the org; callers check access).
func (d *driveFiles) read(ctx context.Context, org, fileID string) ([]byte, string, string, error) {
	f, err := d.drive.Get(ctx, org, fileID)
	if err != nil || f.StorageKey == nil {
		return nil, "", "", fmt.Errorf("file not found")
	}
	rc, _, _, err := d.blobs.Get(ctx, *f.StorageKey)
	if err != nil {
		return nil, "", "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, 60<<20))
	return b, f.Name, f.MimeType, err
}

func (s *Set) wireContracts(c Config) {
	d := &s.Contracts.Deps
	if s.files != nil {
		d.FileMeta = func(ctx context.Context, org, user, id string) (string, error) {
			n, _, _, err := s.files.meta(ctx, org, user, id)
			return n, err
		}
	}
	if c.Contacts != nil {
		d.PartyNames = func(ctx context.Context, org string, ids []string) map[string]string {
			out := map[string]string{}
			list, err := c.Contacts.GetMany(ctx, org, ids)
			if err != nil {
				return out
			}
			for _, p := range list {
				n := p.DisplayName
				if n == "" {
					n = strings.TrimSpace(p.FirstName + " " + p.LastName)
				}
				if n == "" {
					n = p.Company
				}
				out[p.ID] = n
			}
			return out
		}
	}
	if c.Calendar != nil {
		cal := c.Calendar
		d.Calendar = func(ctx context.Context, org, owner, id, title, desc string, day contracts.Date) (string, error) {
			t, err := time.Parse("2006-01-02", day)
			if err != nil {
				return "", err
			}
			f := calendar.Fields{Title: title, Description: desc, StartAt: t, EndAt: t.AddDate(0, 0, 1), AllDay: true, ItemType: calendar.ItemTypeEvent,
				Status: calendar.StatusFree, Visibility: calendar.VisibilityDefault, Reminders: []int32{1440 * 7, 1440}}
			if id != "" {
				if ev, err := cal.Update(ctx, org, id, f); err == nil {
					return ev.ID, nil
				}
			}
			ev, err := cal.Create(ctx, org, owner, f)
			return ev.ID, err
		}
		d.DeleteEvent = func(ctx context.Context, org, id string) error { return cal.Delete(ctx, org, id) }
	}
	if c.Tasks != nil {
		tr := c.Tasks
		d.Task = func(ctx context.Context, org, assignee, id, title, notes string, due contracts.Date, done bool) (string, error) {
			t, err := time.Parse("2006-01-02", due)
			if err != nil {
				return "", err
			}
			if id != "" {
				if cur, err := tr.GetTask(ctx, org, id); err == nil {
					if _, err := tr.UpdateTask(ctx, org, id, tasks.TaskFields{Title: title, Notes: notes, DueAt: &t}); err == nil {
						_ = cur
						return id, nil
					}
				}
			}
			lists, err := tr.ListLists(ctx, org, assignee)
			if err != nil {
				return "", err
			}
			listID := ""
			for _, l := range lists {
				if l.Name == "Contracts" {
					listID = l.ID
					break
				}
			}
			if listID == "" {
				l, err := tr.CreateList(ctx, org, assignee, tasks.ListFields{Name: "Contracts"})
				if err != nil {
					return "", err
				}
				listID = l.ID
			}
			nt, err := tr.CreateTask(ctx, org, listID, assignee, tasks.TaskFields{Title: title, Notes: notes, DueAt: &t})
			return nt.ID, err
		}
		d.DeleteTask = func(ctx context.Context, org, id string) error { return tr.DeleteTask(ctx, org, id) }
	}
}

// PublicHandlers are the handlers that must be reachable without a session (signing links).
func (s *Set) PublicHandlers() []knowkit.Public {
	return []knowkit.Public{signatures.NewPublicHandler(s.Signatures)}
}

func (s *Set) wireSignatures(c Config) {
	d := &s.Signatures.Deps
	if s.files != nil {
		d.ReadFile = func(ctx context.Context, org, owner, id string) ([]byte, string, error) {
			if _, _, _, err := s.files.meta(ctx, org, owner, id); err != nil {
				return nil, "", err
			}
			b, name, _, err := s.files.read(ctx, org, id)
			return b, name, err
		}
		d.ReadFileAny = func(ctx context.Context, org, id string) ([]byte, string, error) {
			b, name, _, err := s.files.read(ctx, org, id)
			return b, name, err
		}
		d.SaveFile = s.files.save
	}
	if c.Mail != nil && c.Mail.Configured() {
		m := c.Mail
		d.Mail = func(ctx context.Context, to []string, subject, html, text string) error {
			return m.Send(ctx, email.Message{To: to, Subject: subject, HTML: html, Text: text})
		}
	}
	if c.Notifications != nil {
		n := c.Notifications
		d.Notify = func(ctx context.Context, org, userID, title, body, link string) {
			if org == "" {
				_ = s.pool.QueryRow(ctx, `SELECT org_id::text FROM adrient.users WHERE id=$1`, userID).Scan(&org)
			}
			if org == "" {
				return
			}
			_, _ = n.Create(ctx, notifications.CreateParams{OrgID: org, UserID: userID, Type: "signature", Title: title, Body: body, TargetURL: link})
		}
	}
}

// wireLegal connects Legal to Drive, Calendar, Tasks, Contacts and Invoicing.
func (s *Set) wireLegal(c Config) {
	d := &s.Legal.Deps
	if s.files != nil {
		d.FileMeta = func(ctx context.Context, org, user, id string) (string, error) {
			n, _, _, err := s.files.meta(ctx, org, user, id)
			return n, err
		}
	}
	if c.Contacts != nil {
		d.Contacts = opsports.Contacts{Repo: c.Contacts}
	}
	if c.Invoicing != nil {
		d.Invoices = opsports.Invoices{Repo: c.Invoicing}
	}
	if c.Calendar != nil {
		cal := c.Calendar
		d.Calendar = func(ctx context.Context, org, owner, id, title, desc, day string) (string, error) {
			t, err := time.Parse("2006-01-02", day)
			if err != nil {
				return "", err
			}
			f := calendar.Fields{Title: title, Description: desc, StartAt: t, EndAt: t.AddDate(0, 0, 1), AllDay: true, ItemType: calendar.ItemTypeEvent,
				Status: calendar.StatusFree, Visibility: calendar.VisibilityDefault, Reminders: []int32{1440 * 7, 1440}}
			if id != "" {
				if ev, err := cal.Update(ctx, org, id, f); err == nil {
					return ev.ID, nil
				}
			}
			ev, err := cal.Create(ctx, org, owner, f)
			return ev.ID, err
		}
		d.DeleteEvent = func(ctx context.Context, org, id string) error { return cal.Delete(ctx, org, id) }
	}
	if c.Tasks != nil {
		tr := c.Tasks
		d.Task = func(ctx context.Context, org, assignee, id, title, notes, due string, done bool) (string, error) {
			t, err := time.Parse("2006-01-02", due)
			if err != nil {
				return "", err
			}
			if id != "" {
				if _, err := tr.GetTask(ctx, org, id); err == nil {
					if _, err := tr.UpdateTask(ctx, org, id, tasks.TaskFields{Title: title, Notes: notes, DueAt: &t}); err == nil {
						return id, nil
					}
				}
			}
			lists, err := tr.ListLists(ctx, org, assignee)
			if err != nil {
				return "", err
			}
			listID := ""
			for _, l := range lists {
				if l.Name == "Legal" {
					listID = l.ID
					break
				}
			}
			if listID == "" {
				l, err := tr.CreateList(ctx, org, assignee, tasks.ListFields{Name: "Legal"})
				if err != nil {
					return "", err
				}
				listID = l.ID
			}
			nt, err := tr.CreateTask(ctx, org, listID, assignee, tasks.TaskFields{Title: title, Notes: notes, DueAt: &t})
			return nt.ID, err
		}
		d.DeleteTask = func(ctx context.Context, org, id string) error { return tr.DeleteTask(ctx, org, id) }
	}
}
