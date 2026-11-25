package cloudimport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Counter tracks one kind's progress. In a dry run Imported means "would import".
type Counter struct {
	Seen     int    `json:"seen"`
	Imported int    `json:"imported"`
	Skipped  int    `json:"skipped"` // already imported on an earlier run, unchanged
	Failed   int    `json:"failed"`
	Bytes    int64  `json:"bytes,omitempty"`
	Note     string `json:"note,omitempty"`
}

// State persists run state. Implemented over Postgres by Wizard and in memory
// in tests. Every method is scoped by connection id, which is itself org-scoped.
type State interface {
	GetMap(ctx context.Context, connID, kind, remoteID string) (local, fingerprint string, ok bool, err error)
	PutMap(ctx context.Context, connID, kind, remoteID, local, fingerprint string) error
	// UserMapping returns the stored decision for a remote user.
	UserMapping(ctx context.Context, connID, remoteID string) (localID string, skip, known bool, err error)
	SaveRemoteUser(ctx context.Context, connID string, u RemoteUser, localID string) error
	SaveProgress(ctx context.Context, runID string, c map[string]*Counter) error
	AddError(ctx context.Context, runID, kind, remoteID, label, msg string) error
}

// RunConfig describes one run.
type RunConfig struct {
	RunID        string
	OrgID        string
	ConnID       string
	ActingUserID string // the signed-in user that started the run
	Scope        string // "me": the source account is the acting user's own; "org": all users
	SelfEmail    string // scope=me: the source account's e-mail
	Kinds        []Kind
	DryRun       bool
	// RootFolder, when set, is a Drive folder created per user to hold the
	// imported files (Drive's root is shared by the whole organisation).
	RootFolder string
}

// Engine runs a migration. It is the only place that decides idempotency,
// ordering and error policy; sources and sinks stay dumb.
type Engine struct {
	Src  Source
	Sink Sink
	St   State

	mu       sync.Mutex
	counters map[string]*Counter
	lastSave time.Time
}

func (e *Engine) ctr(kind Kind) *Counter {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.counters[string(kind)]
	if c == nil {
		c = &Counter{}
		e.counters[string(kind)] = c
	}
	return c
}

func (e *Engine) save(ctx context.Context, cfg RunConfig, force bool) {
	if !force && time.Since(e.lastSave) < 500*time.Millisecond {
		return
	}
	e.lastSave = time.Now()
	e.mu.Lock()
	cp := make(map[string]*Counter, len(e.counters))
	for k, v := range e.counters {
		c := *v
		cp[k] = &c
	}
	e.mu.Unlock()
	_ = e.St.SaveProgress(ctx, cfg.RunID, cp)
}

func (e *Engine) fail(ctx context.Context, cfg RunConfig, kind Kind, remoteID, label string, err error) {
	e.ctr(kind).Failed++
	_ = e.St.AddError(ctx, cfg.RunID, string(kind), remoteID, label, err.Error())
}

func has(ks []Kind, k Kind) bool {
	for _, x := range ks {
		if x == k {
			return true
		}
	}
	return false
}

type localUser struct {
	remote RemoteUser
	id     string
}

// Run executes the run. Counters are saved periodically and at the end, so a
// crash or cancel leaves accurate progress and a later Run resumes by skipping
// everything already in the import map.
func (e *Engine) Run(ctx context.Context, cfg RunConfig) (map[string]*Counter, error) {
	e.counters = map[string]*Counter{}
	defer func() { e.save(context.Background(), cfg, true) }()
	caps := map[Kind]bool{}
	for _, k := range e.Src.Capabilities() {
		caps[k] = true
	}
	for _, k := range cfg.Kinds {
		if !caps[k] {
			e.ctr(k).Note = "not supported by this provider"
		}
	}

	// ---- 1. users: discover and map ---------------------------------------
	var users []localUser
	if cfg.Scope == "me" || cfg.Scope == "" {
		me := RemoteUser{ID: "me", Email: cfg.SelfEmail, Name: cfg.SelfEmail}
		users = append(users, localUser{me, cfg.ActingUserID})
	} else if caps[MigUsers] {
		err := e.Src.Users(ctx, func(ru RemoteUser) error {
			e.ctr(MigUsers).Seen++
			id, skip, known, err := e.St.UserMapping(ctx, cfg.ConnID, ru.ID)
			if err != nil {
				return err
			}
			if !known || (id == "" && !skip) {
				if ru.Email != "" {
					if lid, ok, err := e.Sink.FindUserByEmail(ctx, cfg.OrgID, ru.Email); err == nil && ok {
						id = lid
					}
				}
				if err := e.St.SaveRemoteUser(ctx, cfg.ConnID, ru, id); err != nil {
					return err
				}
			}
			switch {
			case skip:
				e.ctr(MigUsers).Skipped++
			case id == "":
				e.ctr(MigUsers).Failed++
				_ = e.St.AddError(ctx, cfg.RunID, "users", ru.ID, ru.Email, "no Adrient account with this e-mail; create the account (or map it by hand) and run again")
			default:
				e.ctr(MigUsers).Imported++ // "mapped"
				users = append(users, localUser{ru, id})
			}
			return nil
		})
		if err != nil {
			return e.counters, fmt.Errorf("list users: %w", err)
		}
	} else {
		return e.counters, errors.New("this provider cannot list users; connect with scope \"me\"")
	}
	e.save(ctx, cfg, true)

	// ---- 2. groups --------------------------------------------------------
	if has(cfg.Kinds, MigGroups) && caps[MigGroups] && cfg.Scope == "org" {
		err := e.Src.Groups(ctx, func(g RemoteGroup) error {
			c := e.ctr(MigGroups)
			c.Seen++
			if _, _, ok, _ := e.St.GetMap(ctx, cfg.ConnID, "group", g.ID); ok {
				c.Skipped++
				return nil
			}
			if cfg.DryRun {
				c.Imported++
				return nil
			}
			var members []string
			for _, em := range g.Members {
				if id, ok, err := e.Sink.FindUserByEmail(ctx, cfg.OrgID, em); err == nil && ok {
					members = append(members, id)
				}
			}
			id, err := e.Sink.EnsureGroup(ctx, cfg.OrgID, cfg.ActingUserID, g.Name, g.Email, members)
			if err != nil {
				e.fail(ctx, cfg, MigGroups, g.ID, g.Name, err)
				return nil
			}
			_ = e.St.PutMap(ctx, cfg.ConnID, "group", g.ID, id, "")
			c.Imported++
			return nil
		})
		if err != nil && !errors.Is(err, ErrUnsupported) {
			if ctx.Err() != nil {
				return e.counters, ctx.Err()
			}
			e.fail(ctx, cfg, MigGroups, "", "list groups", err)
		}
		e.save(ctx, cfg, true)
	}

	// ---- 3. per-user data -------------------------------------------------
	for _, u := range users {
		for _, k := range []Kind{MigFiles, MigMail, MigCalendar, MigContacts, MigChat} {
			if !has(cfg.Kinds, k) || !caps[k] {
				continue
			}
			if err := ctx.Err(); err != nil {
				return e.counters, err
			}
			var err error
			switch k {
			case MigFiles:
				err = e.files(ctx, cfg, u)
			case MigMail:
				err = e.mail(ctx, cfg, u)
			case MigCalendar:
				err = e.events(ctx, cfg, u)
			case MigContacts:
				err = e.contacts(ctx, cfg, u)
			case MigChat:
				err = e.chat(ctx, cfg, u)
			}
			if err != nil && !errors.Is(err, ErrUnsupported) {
				if ctx.Err() != nil {
					return e.counters, ctx.Err()
				}
				e.fail(ctx, cfg, k, "", "reading "+string(k)+" for "+u.remote.Email, err)
			}
			e.save(ctx, cfg, true)
		}
	}
	return e.counters, nil
}

// mapKey namespaces a remote id by the local owner: the same remote file can be
// visible to several users and must be imported once per owner.
func mapKey(owner, remote string) string { return owner + "|" + remote }

func (e *Engine) files(ctx context.Context, cfg RunConfig, u localUser) error {
	c := e.ctr(MigFiles)
	root := ""
	if cfg.RootFolder != "" && !cfg.DryRun {
		name := cfg.RootFolder
		if u.remote.Email != "" && cfg.Scope == "org" {
			name += " (" + u.remote.Email + ")"
		}
		var err error
		if root, err = e.Sink.EnsureFolder(ctx, cfg.OrgID, u.id, "", safeName(name)); err != nil {
			return err
		}
	}
	return e.Src.Files(ctx, u.remote, func(f RemoteFile) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.Seen++
		defer e.save(ctx, cfg, false)
		key := mapKey(u.id, f.ID)
		_, _, done, err := e.St.GetMap(ctx, cfg.ConnID, "file", key)
		if err != nil {
			return err
		}
		if done {
			c.Skipped++
			return nil
		}
		parent := root
		if f.ParentID != "" {
			p, _, ok, err := e.St.GetMap(ctx, cfg.ConnID, "file", mapKey(u.id, f.ParentID))
			if err != nil {
				return err
			}
			if !ok && !cfg.DryRun {
				e.fail(ctx, cfg, MigFiles, f.ID, f.Name, errors.New("parent folder was not imported"))
				return nil
			}
			parent = p
		}
		if cfg.DryRun {
			c.Imported++
			c.Bytes += f.Size
			return nil
		}
		var local string
		if f.IsDir {
			local, err = e.Sink.EnsureFolder(ctx, cfg.OrgID, u.id, parent, safeName(f.Name))
		} else {
			if f.Open == nil {
				e.fail(ctx, cfg, MigFiles, f.ID, f.Name, errors.New("no content available"))
				return nil
			}
			var rc io.ReadCloser
			rc, err = f.Open(ctx)
			if err == nil {
				local, err = e.Sink.PutFile(ctx, cfg.OrgID, u.id, parent, safeName(f.Name), f.Mime, f.Size, rc)
				rc.Close()
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			e.fail(ctx, cfg, MigFiles, f.ID, f.Name, err)
			return nil
		}
		if err := e.St.PutMap(ctx, cfg.ConnID, "file", key, local, f.Fingerprint); err != nil {
			return err
		}
		for _, s := range f.Shares {
			if s.Email == "" {
				continue
			}
			if gid, ok, err := e.Sink.FindUserByEmail(ctx, cfg.OrgID, s.Email); err == nil && ok && gid != u.id {
				if err := e.Sink.ShareFile(ctx, cfg.OrgID, u.id, local, gid, normRole(s.Role)); err != nil {
					e.fail(ctx, cfg, MigFiles, f.ID, "share "+f.Name+" with "+s.Email, err)
				}
			}
		}
		c.Imported++
		c.Bytes += f.Size
		return nil
	})
}

func (e *Engine) mail(ctx context.Context, cfg RunConfig, u localUser) error {
	c := e.ctr(MigMail)
	return e.Src.Mail(ctx, u.remote, func(m RemoteMail) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.Seen++
		defer e.save(ctx, cfg, false)
		key := mapKey(u.id, m.ID)
		if _, _, ok, err := e.St.GetMap(ctx, cfg.ConnID, "mail", key); err != nil {
			return err
		} else if ok {
			c.Skipped++
			return nil
		}
		if cfg.DryRun {
			c.Imported++
			c.Bytes += int64(len(m.Raw))
			return nil
		}
		mf, err := ParseMail(m)
		if err != nil {
			e.fail(ctx, cfg, MigMail, m.ID, m.Folder, err)
			return nil
		}
		// Message-ID de-dup: the same message seen in another folder/label (Gmail
		// labels, re-exported mailboxes) is stored once and gains the labels.
		midKey := ""
		if mf.MessageID != "" {
			midKey = mapKey(u.id, mf.MessageID)
			if local, _, ok, _ := e.St.GetMap(ctx, cfg.ConnID, "mailmid", midKey); ok {
				if len(mf.Labels) > 0 && local != "" {
					if err := e.Sink.LabelMail(ctx, cfg.OrgID, u.id, local, mf.Labels); err != nil {
						e.fail(ctx, cfg, MigMail, m.ID, mf.Subject, err)
						return nil
					}
				}
				_ = e.St.PutMap(ctx, cfg.ConnID, "mail", key, local, "")
				c.Skipped++
				return nil
			}
		}
		id, err := e.Sink.AddMail(ctx, cfg.OrgID, u.id, mf)
		if err != nil {
			e.fail(ctx, cfg, MigMail, m.ID, mf.Subject, err)
			return nil
		}
		_ = e.St.PutMap(ctx, cfg.ConnID, "mail", key, id, "")
		if midKey != "" {
			_ = e.St.PutMap(ctx, cfg.ConnID, "mailmid", midKey, id, "")
		}
		c.Imported++
		c.Bytes += int64(len(m.Raw))
		return nil
	})
}

func (e *Engine) events(ctx context.Context, cfg RunConfig, u localUser) error {
	c := e.ctr(MigCalendar)
	return e.Src.Events(ctx, u.remote, func(ev RemoteEvent) error {
		c.Seen++
		defer e.save(ctx, cfg, false)
		key := mapKey(u.id, ev.ID)
		if _, _, ok, err := e.St.GetMap(ctx, cfg.ConnID, "event", key); err != nil {
			return err
		} else if ok {
			c.Skipped++
			return nil
		}
		if cfg.DryRun {
			c.Imported++
			return nil
		}
		id, err := e.Sink.AddEvent(ctx, cfg.OrgID, u.id, ev.Fields)
		if err != nil {
			e.fail(ctx, cfg, MigCalendar, ev.ID, ev.Fields.Title, err)
			return nil
		}
		_ = e.St.PutMap(ctx, cfg.ConnID, "event", key, id, ev.Fingerprint)
		c.Imported++
		return nil
	})
}

func (e *Engine) contacts(ctx context.Context, cfg RunConfig, u localUser) error {
	c := e.ctr(MigContacts)
	return e.Src.Contacts(ctx, u.remote, func(ct RemoteContact) error {
		c.Seen++
		defer e.save(ctx, cfg, false)
		key := mapKey(u.id, ct.ID)
		if _, _, ok, err := e.St.GetMap(ctx, cfg.ConnID, "contact", key); err != nil {
			return err
		} else if ok {
			c.Skipped++
			return nil
		}
		if cfg.DryRun {
			c.Imported++
			return nil
		}
		id, err := e.Sink.AddContact(ctx, cfg.OrgID, u.id, ct.Fields)
		if err != nil {
			e.fail(ctx, cfg, MigContacts, ct.ID, ct.Fields.DisplayName, err)
			return nil
		}
		_ = e.St.PutMap(ctx, cfg.ConnID, "contact", key, id, ct.Fingerprint)
		c.Imported++
		return nil
	})
}

func (e *Engine) chat(ctx context.Context, cfg RunConfig, u localUser) error {
	c := e.ctr(MigChat)
	return e.Src.Chat(ctx, u.remote, func(m RemoteChat) error {
		c.Seen++
		defer e.save(ctx, cfg, false)
		key := mapKey(u.id, m.ID)
		if _, _, ok, err := e.St.GetMap(ctx, cfg.ConnID, "chat", key); err != nil {
			return err
		} else if ok {
			c.Skipped++
			return nil
		}
		if cfg.DryRun {
			c.Imported++
			return nil
		}
		id, err := e.Sink.AddChat(ctx, cfg.OrgID, u.id, m)
		if err != nil {
			e.fail(ctx, cfg, MigChat, m.ID, m.Space, err)
			return nil
		}
		_ = e.St.PutMap(ctx, cfg.ConnID, "chat", key, id, "")
		c.Imported++
		return nil
	})
}

func normRole(r string) string {
	switch strings.ToLower(r) {
	case "editor", "writer", "owner", "write", "edit":
		return "editor"
	case "commenter":
		return "commenter"
	}
	return "viewer"
}

// safeName makes a remote name usable as a Drive name.
func safeName(n string) string {
	n = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 || r < 32 {
			return '_'
		}
		return r
	}, n)
	n = strings.TrimSpace(n)
	if n == "" || n == "." || n == ".." {
		return "unnamed"
	}
	return n
}
