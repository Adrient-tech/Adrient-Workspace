package crm

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

// ExtractAddrs lower-cases and collects addresses from a from field and JSON arrays of recipients.
func ExtractAddrs(from string, lists ...[]byte) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = NormEmail(s)
		if i := strings.LastIndex(s, "<"); i >= 0 && strings.HasSuffix(s, ">") {
			s = s[i+1 : len(s)-1]
		}
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(from)
	for _, l := range lists {
		var arr []string
		_ = json.Unmarshal(l, &arr)
		for _, a := range arr {
			add(a)
		}
	}
	return out
}

// SyncMail matches the user's mailbox (last 90 days, inbox and sent) against
// CRM people by e-mail address and records metadata (subject, snippet, thread
// id) as activities and mail-log rows. Message bodies are never copied.
// Running it again logs only messages not logged before.
func (r *Repository) SyncMail(ctx context.Context, org, user string) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, p.contact_id::text, c.emails FROM adrient.crm_people p
		JOIN adrient.contacts c ON c.id=p.contact_id WHERE p.org_id=$1 AND c.trashed_at IS NULL AND `+ViewerFrom(ctx).ownerSQL("p"), org)
	if err != nil {
		return 0, err
	}
	type pc struct{ person, contact string }
	byAddr := map[string][]pc{}
	for rows.Next() {
		var p, c string
		var e []byte
		if err := rows.Scan(&p, &c, &e); err != nil {
			rows.Close()
			return 0, err
		}
		var es []string
		_ = json.Unmarshal(e, &es)
		for _, a := range es {
			byAddr[NormEmail(a)] = append(byAddr[NormEmail(a)], pc{p, c})
		}
	}
	rows.Close()
	if len(byAddr) == 0 {
		return 0, nil
	}
	mrows, err := r.pool.Query(ctx, `SELECT m.id::text, m.thread_id::text, m.folder, m.from_addr, m.to_addrs, m.cc_addrs, m.subject, m.snippet, m.sent_at
		FROM adrient.mail_messages m WHERE m.org_id=$1 AND m.owner_id=$2 AND m.folder IN ('inbox','sent') AND m.sent_at > now() - interval '90 days'
		AND NOT EXISTS (SELECT 1 FROM adrient.crm_mail_log l WHERE l.message_id=m.id) ORDER BY m.sent_at LIMIT 2000`, org, user)
	if err != nil {
		return 0, err
	}
	type msg struct {
		id, thread, folder, from, subject, snippet string
		to, cc                                     []byte
		at                                         time.Time
	}
	var msgs []msg
	for mrows.Next() {
		var m msg
		if err := mrows.Scan(&m.id, &m.thread, &m.folder, &m.from, &m.to, &m.cc, &m.subject, &m.snippet, &m.at); err != nil {
			mrows.Close()
			return 0, err
		}
		msgs = append(msgs, m)
	}
	mrows.Close()
	logged := 0
	for _, m := range msgs {
		dir := "in"
		if m.folder == "sent" {
			dir = "out"
		}
		seen := map[string]bool{}
		for _, a := range ExtractAddrs(m.from, m.to, m.cc) {
			for _, p := range byAddr[a] {
				if seen[p.person] {
					continue
				}
				seen[p.person] = true
				ct, err := r.pool.Exec(ctx, `INSERT INTO adrient.crm_mail_log (org_id,message_id,thread_id,person_id,user_id,direction,subject,snippet,sent_at)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (message_id,person_id) DO NOTHING`, org, m.id, m.thread, p.person, user, dir, m.subject, truncate(m.snippet, 200), m.at)
				if err != nil {
					return logged, err
				}
				if ct.RowsAffected() == 0 {
					continue
				}
				logged++
				// A mail sent from the CRM already wrote its own activity; do not add a second one.
				_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.crm_activities (org_id,kind,summary,occurred_at,contact_id,mail_thread_id,direction,user_id)
					SELECT $1,'email',$2,$3,$4,$5,$6,$7 WHERE NOT EXISTS (SELECT 1 FROM adrient.crm_activities a WHERE a.org_id=$1 AND a.kind='email'
					AND a.contact_id=$4 AND a.mail_thread_id=$5 AND a.direction=$6 AND abs(extract(epoch FROM a.occurred_at - $3::timestamptz)) < 180)`,
					org, firstNonEmpty(m.subject, "(no subject)"), m.at, p.contact, m.thread, dir, user)
				if dir == "in" {
					r.fire(ctx, Event{Org: org, Trigger: TrigEmailReceived, PersonID: p.person, FromAddr: NormEmail(m.from), Key: "mail:" + m.id + ":" + p.person})
				}
			}
		}
	}
	return logged, nil
}

// AutoLogMail runs SyncMail for every user who turned on automatic logging.
func (r *Repository) AutoLogMail(ctx context.Context) {
	rows, err := r.pool.Query(ctx, `SELECT org_id::text, user_id::text, scope FROM adrient.crm_user_scope WHERE auto_log_mail`)
	if err != nil {
		return
	}
	type u struct{ org, user, scope string }
	var us []u
	for rows.Next() {
		var x u
		_ = rows.Scan(&x.org, &x.user, &x.scope)
		us = append(us, x)
	}
	rows.Close()
	for _, x := range us {
		c := WithViewer(ctx, Viewer{UserID: x.user, Scope: x.scope})
		if _, err := r.SyncMail(c, x.org, x.user); err != nil {
			slog.Warn("crm auto mail log", "err", err)
		}
	}
}

// SendInput is a mail sent from the CRM (optionally from a template).
type SendInput struct {
	PersonID   string `json:"person_id"`
	DealID     string `json:"deal_id"`
	TemplateID string `json:"template_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
}

// SendFromCRM sends a mail to a person as the signed-in user (through the Mail
// app, so it appears in Sent) and records it on the person. Template
// placeholders such as {{person.name}} are filled in. No open tracking is added.
func (r *Repository) SendFromCRM(ctx context.Context, org string, in SendInput) (string, error) {
	if r.hooks == nil || r.hooks.SendAsUser == nil {
		return "", invalid("sending mail is not available")
	}
	ec := r.loadContext(ctx, Event{Org: org, PersonID: in.PersonID, DealID: in.DealID})
	if ec.Email == "" {
		return "", invalid("this person has no email address")
	}
	subj, body := in.Subject, in.Body
	if in.TemplateID != "" {
		var ts, tb string
		if err := r.pool.QueryRow(ctx, `SELECT subject, body FROM adrient.crm_email_templates WHERE org_id=$1 AND id=$2`, org, in.TemplateID).Scan(&ts, &tb); err != nil {
			return "", ErrNotFound
		}
		if strings.TrimSpace(subj) == "" {
			subj = ts
		}
		if strings.TrimSpace(body) == "" {
			body = tb
		}
	}
	vars := ec.vars()
	subj, body = RenderTemplate(subj, vars), RenderTemplate(body, vars)
	if strings.TrimSpace(subj) == "" {
		return "", invalid("a subject is required")
	}
	thread, err := r.hooks.SendAsUser(ctx, []string{ec.Email}, subj, body)
	if err != nil {
		return "", err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.crm_activities (org_id,kind,summary,contact_id,deal_id,mail_thread_id,direction,user_id)
		VALUES ($1,'email',$2,$3,$4,$5,'out',$6)`, org, subj, nz(ec.ContactID), nz(in.DealID), thread, uidOrNil(ctx))
	return thread, err
}
