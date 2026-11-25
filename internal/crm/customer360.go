package crm

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Link is a deep link into another Adrient app.
type Link struct {
	App   string `json:"app"`
	Label string `json:"label"`
	Href  string `json:"href"`
}

// Customer360 is everything the workspace knows about a company or a person.
type Customer360 struct {
	Kind        string           `json:"kind"`
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Emails      []string         `json:"emails"`
	Phones      []string         `json:"phones"`
	People      []map[string]any `json:"people"`
	Deals       []map[string]any `json:"deals"`
	Followups   []map[string]any `json:"followups"`
	Activities  []map[string]any `json:"activities"`
	Meetings    []map[string]any `json:"meetings"`
	Mails       []map[string]any `json:"mails"`
	Invoices    []map[string]any `json:"invoices"`
	Tickets     []map[string]any `json:"tickets"`
	Projects    []map[string]any `json:"projects"`
	Files       []map[string]any `json:"files"`
	Paid        float64          `json:"paid_total"`
	Outstanding float64          `json:"outstanding_total"`
	Links       []Link           `json:"links"`
	Notes       []string         `json:"notes"`
}

func (r *Repository) rows(ctx context.Context, q string, args ...any) []map[string]any {
	out := []map[string]any{}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	names := rows.FieldDescriptions()
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return out
		}
		m := map[string]any{}
		for i, fd := range names {
			v := vals[i]
			switch t := v.(type) {
			case [16]byte:
				v = uuidString(t)
			case time.Time:
				v = t.Format(time.RFC3339)
			case []byte:
				var j any
				if json.Unmarshal(t, &j) == nil {
					v = j
				}
			}
			m[string(fd.Name)] = v
		}
		out = append(out, m)
	}
	return out
}

func uuidString(b [16]byte) string {
	const hexd = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, x := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hexd[x>>4], hexd[x&15])
	}
	return string(out)
}

// BuildCustomer360 gathers the 360 view. kind is "company" or "person" (a
// crm_people id). It respects the viewer's visibility for CRM records, and
// shows only the viewer's own logged mail and Drive files.
func (r *Repository) BuildCustomer360(ctx context.Context, org, kind, id string) (Customer360, error) {
	v := ViewerFrom(ctx)
	c := Customer360{Kind: kind, ID: id, Emails: []string{}, Phones: []string{}, Notes: []string{}}
	var contactIDs []string
	var emails []string
	var search string
	switch kind {
	case "company":
		var n string
		err := r.pool.QueryRow(ctx, `SELECT name FROM adrient.crm_companies co WHERE org_id=$1 AND id=$2 AND `+v.ownerSQL("co"), org, id).Scan(&n)
		if err != nil {
			return c, ErrNotFound
		}
		c.Name, search = n, n
		c.People = r.rows(ctx, `SELECT p.id::text AS id, ct.display_name AS name, ct.emails, ct.phones, p.status, p.score FROM adrient.crm_people p
			JOIN adrient.contacts ct ON ct.id=p.contact_id WHERE p.org_id=$1 AND p.company_id=$2 AND ct.trashed_at IS NULL AND `+v.ownerSQL("p")+` ORDER BY ct.display_name`, org, id)
		rows, _ := r.pool.Query(ctx, `SELECT ct.id::text, ct.emails, ct.phones FROM adrient.crm_people p JOIN adrient.contacts ct ON ct.id=p.contact_id
			WHERE p.org_id=$1 AND p.company_id=$2 AND ct.trashed_at IS NULL AND `+v.ownerSQL("p"), org, id)
		for rows != nil && rows.Next() {
			var cid string
			var e, p []byte
			_ = rows.Scan(&cid, &e, &p)
			contactIDs = append(contactIDs, cid)
			var es []string
			_ = json.Unmarshal(e, &es)
			emails = append(emails, es...)
		}
		if rows != nil {
			rows.Close()
		}
		c.Deals = r.rows(ctx, `SELECT d.id::text AS id, d.title, d.stage, d.value::float8 AS value, d.currency, d.expected_close FROM adrient.crm_deals d
			WHERE d.org_id=$1 AND d.company_id=$2 AND `+v.ownerSQL("d")+` ORDER BY d.updated_at DESC`, org, id)
		c.Activities = r.rows(ctx, `SELECT a.id::text AS id, a.kind, a.summary, a.occurred_at, a.direction FROM adrient.crm_activities a WHERE a.org_id=$1
			AND (a.company_id=$2 OR a.contact_id=ANY($3::uuid[]) OR a.deal_id IN (SELECT id FROM adrient.crm_deals WHERE company_id=$2)) AND `+v.childSQL("a")+` ORDER BY a.occurred_at DESC LIMIT 100`, org, id, nonNilStrs(contactIDs))
		c.Links = append(c.Links, Link{"crm", "Open company in CRM", "/crm?company=" + id})
	case "person":
		var cid, name string
		var e, p []byte
		err := r.pool.QueryRow(ctx, `SELECT ct.id::text, ct.display_name, ct.emails, ct.phones FROM adrient.crm_people p JOIN adrient.contacts ct ON ct.id=p.contact_id
			WHERE p.org_id=$1 AND p.id=$2 AND `+v.ownerSQL("p"), org, id).Scan(&cid, &name, &e, &p)
		if err != nil {
			return c, ErrNotFound
		}
		c.Name = name
		_ = json.Unmarshal(e, &emails)
		_ = json.Unmarshal(p, &c.Phones)
		contactIDs = []string{cid}
		search = name
		c.Deals = r.rows(ctx, `SELECT d.id::text AS id, d.title, d.stage, d.value::float8 AS value, d.currency, d.expected_close FROM adrient.crm_deals d
			WHERE d.org_id=$1 AND (d.contact_id=$2 OR d.id IN (SELECT deal_id FROM adrient.crm_deal_people WHERE contact_id=$2)) AND `+v.ownerSQL("d")+` ORDER BY d.updated_at DESC`, org, cid)
		c.Activities = r.rows(ctx, `SELECT a.id::text AS id, a.kind, a.summary, a.occurred_at, a.direction FROM adrient.crm_activities a WHERE a.org_id=$1 AND a.contact_id=$2
			AND `+v.childSQL("a")+` ORDER BY a.occurred_at DESC LIMIT 100`, org, cid)
		c.Links = append(c.Links, Link{"contacts", "Open contact", "/contacts"})
	default:
		return c, invalid("kind must be company or person")
	}
	c.Emails = nonNilStrs(emails)
	if kind == "person" {
		c.Emails = nonNilStrs(emails)
	}
	c.Followups = r.rows(ctx, `SELECT f.id::text AS id, f.title, f.due_date, f.done FROM adrient.crm_followups f WHERE f.org_id=$1 AND
		(f.contact_id=ANY($2::uuid[]) OR f.deal_id IN (SELECT id FROM adrient.crm_deals WHERE contact_id=ANY($2::uuid[]) OR ($3<>'' AND company_id::text=$3))) AND `+v.childSQL("f")+` ORDER BY f.done, f.due_date LIMIT 50`,
		org, nonNilStrs(contactIDs), map[bool]string{true: id, false: ""}[kind == "company"])
	for _, a := range c.Activities {
		if a["kind"] == "meeting" {
			c.Meetings = append(c.Meetings, a)
		}
	}
	// Mail: metadata logged by the viewer (never other users' mailboxes).
	mailUser := v.UserID
	c.Mails = r.rows(ctx, `SELECT l.thread_id::text AS thread_id, l.subject, l.snippet, l.direction, l.sent_at FROM adrient.crm_mail_log l
		JOIN adrient.crm_people p ON p.id=l.person_id WHERE l.org_id=$1 AND p.contact_id=ANY($2::uuid[]) AND ($3='' OR l.user_id::text=$3) ORDER BY l.sent_at DESC LIMIT 30`,
		org, nonNilStrs(contactIDs), mailUser)
	// Invoicing: by contact id or by the client's e-mail, with payments.
	c.Invoices = r.rows(ctx, `SELECT d.id::text AS id, d.kind, d.number, d.status, d.total::float8 AS total, d.currency, d.issue_date, d.due_date,
		coalesce((SELECT sum(CASE WHEN p.kind='payment' THEN p.amount WHEN p.kind='refund' THEN -p.amount ELSE 0 END) FROM adrient.invc_payments p WHERE p.document_id=d.id),0)::float8 AS paid
		FROM adrient.invc_documents d WHERE d.org_id=$1 AND d.deleted_at IS NULL AND (d.contact_id=ANY($2::uuid[]) OR lower(d.client_email)=ANY($3::text[])
		OR ($4<>'' AND lower(d.client_name)=lower($4))) ORDER BY d.issue_date DESC LIMIT 50`, org, nonNilStrs(contactIDs), lowerAll(emails), companyName(kind, c.Name))
	for _, inv := range c.Invoices {
		if inv["kind"] == "invoice" {
			t, _ := inv["total"].(float64)
			p, _ := inv["paid"].(float64)
			c.Paid += p
			if st, _ := inv["status"].(string); st != "draft" {
				if t-p > 0 {
					c.Outstanding += t - p
				}
			}
		}
	}
	c.Paid, c.Outstanding = round2(c.Paid), round2(c.Outstanding)
	c.Tickets = r.rows(ctx, `SELECT t.id::text AS id, tp.key || '-' || t.number AS key, t.title, t.status, t.priority, t.created_at FROM adrient.tickets t
		JOIN adrient.ticket_projects tp ON tp.id=t.project_id WHERE t.org_id=$1 AND lower(t.requester_email)=ANY($2::text[]) ORDER BY t.created_at DESC LIMIT 30`, org, lowerAll(emails))
	// Projects and Drive have no CRM link field: match by name and say so.
	if len(search) >= 3 {
		pat := "%" + escapeLike(strings.ToLower(search)) + "%"
		c.Projects = r.rows(ctx, `SELECT p.id::text AS id, p.name, p.state FROM adrient.project_projects p WHERE p.org_id=$1 AND p.trashed_at IS NULL AND lower(p.name) LIKE $2 ORDER BY p.updated_at DESC LIMIT 10`, org, pat)
		if v.UserID != "" && uuidRe.MatchString(v.UserID) {
			c.Files = r.rows(ctx, `SELECT f.id::text AS id, f.name, f.mime_type FROM adrient.drive_files f WHERE f.org_id=$1 AND f.owner_id=$3 AND f.trashed_at IS NULL
				AND f.mime_type<>'folder' AND lower(f.name) LIKE $2 ORDER BY f.updated_at DESC LIMIT 10`, org, pat, v.UserID)
		}
		c.Notes = append(c.Notes, "Projects and Drive files are matched by name and by explicit links from won deals. Drive shows only your own files.")
	}
	// Explicit links (record_links): projects made from this customer's deals, tickets linked to the company.
	dealIDs := []string{}
	for _, d := range c.Deals {
		if id, ok := d["id"].(string); ok {
			dealIDs = append(dealIDs, id)
		}
	}
	linked := r.rows(ctx, `SELECT p.id::text AS id, p.name, p.state FROM adrient.project_projects p WHERE p.org_id=$1 AND p.trashed_at IS NULL AND p.id::text IN
		(SELECT to_id FROM adrient.record_links WHERE org_id=$1 AND to_app='projects' AND to_type='project' AND from_app='crm' AND from_type='deal' AND from_id=ANY($2::text[]))`, org, dealIDs)
	c.Projects = mergeByID(c.Projects, linked)
	linkedT := r.rows(ctx, `SELECT t.id::text AS id, tp.key || '-' || t.number AS key, t.title, t.status, t.priority, t.created_at FROM adrient.tickets t
		JOIN adrient.ticket_projects tp ON tp.id=t.project_id WHERE t.org_id=$1 AND t.id::text IN
		(SELECT from_id FROM adrient.record_links WHERE org_id=$1 AND from_app='tickets' AND from_type='ticket' AND to_app='crm' AND to_type=$3 AND to_id=$2)`, org, id, kind)
	c.Tickets = mergeByID(c.Tickets, linkedT)
	c.Links = append(c.Links,
		Link{"mail", "Open Mail", "/mail"}, Link{"invoicing", "Open Invoicing", "/invoicing"}, Link{"tickets", "Open Tickets", "/tickets"},
		Link{"projects", "Open Projects", "/projects"}, Link{"drive", "Open Drive", "/drive"}, Link{"calendar", "Open Calendar", "/calendar"})
	nilFix := func(s *[]map[string]any) {
		if *s == nil {
			*s = []map[string]any{}
		}
	}
	for _, s := range []*[]map[string]any{&c.People, &c.Deals, &c.Followups, &c.Activities, &c.Meetings, &c.Mails, &c.Invoices, &c.Tickets, &c.Projects, &c.Files} {
		nilFix(s)
	}
	return c, nil
}

func companyName(kind, name string) string {
	if kind == "company" {
		return name
	}
	return ""
}

func lowerAll(ss []string) []string {
	out := []string{}
	for _, s := range ss {
		out = append(out, strings.ToLower(strings.TrimSpace(s)))
	}
	return out
}

// mergeByID appends rows of b whose id is not already in a.
func mergeByID(a, b []map[string]any) []map[string]any {
	seen := map[any]bool{}
	for _, x := range a {
		seen[x["id"]] = true
	}
	for _, x := range b {
		if !seen[x["id"]] {
			a = append(a, x)
		}
	}
	return a
}
