package legal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

// ---- documents (links to Drive, Signatures and Contracts) ----------------------------------------------------

// Document links a matter to a Drive file, a signature request or a contract.
type Document struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"` // drive | signature | contract
	RefID    string `json:"ref_id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status,omitempty"`
}

func (r *Repository) documents(ctx context.Context, org, matter string) ([]Document, error) {
	rows, err := r.pool.Query(ctx, `SELECT d.id::text, d.kind, d.ref_id::text, d.name, d.category,
		CASE d.kind WHEN 'signature' THEN COALESCE((SELECT status FROM adrient.sig_requests s WHERE s.id=d.ref_id AND s.org_id=d.org_id),'')
		            WHEN 'contract' THEN COALESCE((SELECT status FROM adrient.contracts_contracts c WHERE c.id=d.ref_id AND c.org_id=d.org_id),'') ELSE '' END
		FROM adrient.legal_documents d WHERE d.org_id=$1 AND d.matter_id=$2 ORDER BY d.created_at`, org, matter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.Kind, &d.RefID, &d.Name, &d.Category, &d.Status); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Lookup is something a matter can link to.
type Lookup struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
}

// Linkable lists the signature requests or contracts the person could link to a matter.
func (r *Repository) Linkable(ctx context.Context, a knowkit.Actor, kind, q string) ([]Lookup, error) {
	var sql string
	switch kind {
	case "signature":
		sql = `SELECT id::text, title, status FROM adrient.sig_requests WHERE org_id=$1 AND ($2='' OR title ILIKE '%'||$2||'%') ORDER BY created_at DESC LIMIT 30`
	case "contract":
		sql = `SELECT id::text, title, status FROM adrient.contracts_contracts WHERE org_id=$1 AND deleted_at IS NULL AND (access='org' OR owner_id=$3 OR $3=ANY(shared_with) OR $4)
			AND ($2='' OR title ILIKE '%'||$2||'%') ORDER BY updated_at DESC LIMIT 30`
	default:
		return nil, knowkit.Invalid("kind must be signature or contract")
	}
	args := []any{a.Org, q}
	if kind == "contract" {
		args = append(args, a.User, a.Admin)
	}
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lookup{}
	for rows.Next() {
		var l Lookup
		if err := rows.Scan(&l.ID, &l.Name, &l.Status); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DocInput links a document.
type DocInput struct {
	Kind     string `json:"kind"`
	RefID    string `json:"ref_id"`
	Category string `json:"category"`
}

// AddDocument links a Drive file, signature request or contract to a matter.
func (r *Repository) AddDocument(ctx context.Context, a knowkit.Actor, matterID string, in DocInput) (Document, error) {
	if _, err := r.loadMatter(ctx, a, matterID); err != nil {
		return Document{}, err
	}
	if !knowkit.IsUUID(in.RefID) || tooLong(in.Category, 60) {
		return Document{}, knowkit.Invalid("a document needs a valid id")
	}
	name := ""
	switch in.Kind {
	case "drive":
		if r.Deps.FileMeta == nil {
			return Document{}, knowkit.ErrUnavailable
		}
		n, err := r.Deps.FileMeta(ctx, a.Org, a.User, in.RefID)
		if err != nil {
			return Document{}, knowkit.Invalid("%v", err)
		}
		name = n
	case "signature":
		if err := r.pool.QueryRow(ctx, `SELECT title FROM adrient.sig_requests WHERE org_id=$1 AND id=$2`, a.Org, in.RefID).Scan(&name); err != nil {
			return Document{}, knowkit.ErrNotFound
		}
	case "contract":
		if err := r.pool.QueryRow(ctx, `SELECT title FROM adrient.contracts_contracts WHERE org_id=$1 AND id=$2 AND deleted_at IS NULL AND (access='org' OR owner_id=$3 OR $3=ANY(shared_with) OR $4)`,
			a.Org, in.RefID, a.User, a.Admin).Scan(&name); err != nil {
			return Document{}, knowkit.ErrNotFound
		}
	default:
		return Document{}, knowkit.Invalid("kind must be drive, signature or contract")
	}
	var d Document
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.legal_documents (org_id, matter_id, kind, ref_id, name, category, added_by) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (matter_id, kind, ref_id) DO UPDATE SET category=EXCLUDED.category RETURNING id::text, kind, ref_id::text, name, category`,
		a.Org, matterID, in.Kind, in.RefID, name, strings.TrimSpace(in.Category), a.User).Scan(&d.ID, &d.Kind, &d.RefID, &d.Name, &d.Category)
	if err != nil {
		return d, err
	}
	r.audit(ctx, "document_link", "legal_matter", matterID)
	return d, nil
}

// RemoveDocument unlinks a document (the file itself stays where it is).
func (r *Repository) RemoveDocument(ctx context.Context, a knowkit.Actor, matterID, docID string) error {
	if _, err := r.loadMatter(ctx, a, matterID); err != nil {
		return err
	}
	if !knowkit.IsUUID(docID) {
		return knowkit.ErrNotFound
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.legal_documents WHERE org_id=$1 AND matter_id=$2 AND id=$3`, a.Org, matterID, docID)
	r.audit(ctx, "document_unlink", "legal_matter", matterID)
	return err
}

// ---- notes ------------------------------------------------------------------------------------------------

// Note is a note, a logged call, email or meeting, or a saved conflict check.
type Note struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	Body    string    `json:"body"`
	Author  string    `json:"author"`
	Created time.Time `json:"created_at"`
}

// Notes lists a matter's notes, newest first.
func (r *Repository) Notes(ctx context.Context, a knowkit.Actor, matterID string) ([]Note, error) {
	if _, err := r.loadMatter(ctx, a, matterID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT n.id::text, n.kind, n.body, COALESCE(NULLIF(u.display_name,''),u.email,''), n.created_at FROM adrient.legal_notes n
		LEFT JOIN adrient.users u ON u.id=n.author_id WHERE n.org_id=$1 AND n.matter_id=$2 ORDER BY n.created_at DESC LIMIT 500`, a.Org, matterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Kind, &n.Body, &n.Author, &n.Created); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddNote appends a note to a matter.
func (r *Repository) AddNote(ctx context.Context, a knowkit.Actor, matterID, kind, body string) (Note, error) {
	if _, err := r.loadMatter(ctx, a, matterID); err != nil {
		return Note{}, err
	}
	body = strings.TrimSpace(body)
	if kind == "" {
		kind = "note"
	}
	if !knowkit.OneOf(kind, "note", "call", "email", "meeting", "conflict") || body == "" || tooLong(body, 10000) {
		return Note{}, knowkit.Invalid("a note needs text of up to 10,000 characters")
	}
	var n Note
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.legal_notes (org_id, matter_id, kind, body, author_id) VALUES ($1,$2,$3,$4,$5) RETURNING id::text, kind, body, created_at`,
		a.Org, matterID, kind, body, a.User).Scan(&n.ID, &n.Kind, &n.Body, &n.Created)
	r.audit(ctx, "note_add", "legal_matter", matterID)
	return n, err
}

// ---- conflict check -----------------------------------------------------------------------------------------

// ConflictResult is the outcome of a conflict search.
type ConflictResult struct {
	Query string        `json:"query"`
	Hits  []ConflictHit `json:"hits"`
	Clear bool          `json:"clear"`
}

// ConflictHit is a person or company that matches, with where they appear.
type ConflictHit struct {
	Hit
	Matters []ConflictMatter `json:"matters"`
}

// ConflictMatter is a matter a hit appears on. Matters the person may not see show only that one exists.
type ConflictMatter struct {
	ID         string `json:"id,omitempty"`
	Number     string `json:"number,omitempty"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
	Role       string `json:"role"`
	Restricted bool   `json:"restricted,omitempty"`
}

// ConflictCheck searches clients, matter parties and Contacts by name, email or phone (accent and case
// insensitive) and reports every matter each match appears on and in what role.
func (r *Repository) ConflictCheck(ctx context.Context, a knowkit.Actor, query string) (ConflictResult, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || tooLong(query, 200) {
		return ConflictResult{}, knowkit.Invalid("type at least 2 characters to search")
	}
	type mref struct {
		ConflictMatter
		allowed bool
	}
	var cands []Candidate
	appear := map[string][]mref{} // candidate key -> matters
	key := func(src, id string) string { return src + ":" + id }
	addMatter := func(k string, m Matter, role string) {
		appear[k] = append(appear[k], mref{ConflictMatter{ID: m.ID, Number: m.Number, Title: m.Title, Status: m.Status, Role: role}, r.canView(knowkit.Actor{Org: a.Org, User: a.User, Admin: a.Admin}, m)})
	}
	// Clients and the matters they own.
	rows, err := r.pool.Query(ctx, `SELECT c.id::text, c.name, c.email, c.phone, COALESCE(c.contact_id::text,'') FROM adrient.legal_clients c WHERE c.org_id=$1 LIMIT 20000`, a.Org)
	if err != nil {
		return ConflictResult{}, err
	}
	for rows.Next() {
		var id, name, email, phone, contact string
		if rows.Scan(&id, &name, &email, &phone, &contact) == nil {
			cands = append(cands, Candidate{Source: "client", ID: id, Name: name, Extra: []string{email, phone}, Role: "Client"})
		}
	}
	rows.Close()
	mrows, err := r.pool.Query(ctx, `SELECT m.id::text, m.number, m.title, m.status, m.access, COALESCE(m.shared_with::text[],'{}'), COALESCE(m.created_by::text,''), COALESCE(m.responsible_id::text,''), COALESCE(m.originating_id::text,''),
		COALESCE(m.client_id::text,'') FROM adrient.legal_matters m WHERE m.org_id=$1 AND m.deleted_at IS NULL`, a.Org)
	if err != nil {
		return ConflictResult{}, err
	}
	for mrows.Next() {
		var m Matter
		var client string
		if mrows.Scan(&m.ID, &m.Number, &m.Title, &m.Status, &m.Access, &m.SharedWith, &m.CreatedBy, &m.ResponsibleID, &m.OriginatingID, &client) == nil && client != "" {
			addMatter(key("client", client), m, "Client")
		}
	}
	mrows.Close()
	// Related parties.
	prows, err := r.pool.Query(ctx, `SELECT p.id::text, p.name, p.email, p.phone, p.role, m.id::text, m.number, m.title, m.status, m.access, COALESCE(m.shared_with::text[],'{}'), COALESCE(m.created_by::text,''), COALESCE(m.responsible_id::text,''), COALESCE(m.originating_id::text,'')
		FROM adrient.legal_parties p JOIN adrient.legal_matters m ON m.id=p.matter_id WHERE p.org_id=$1 AND m.deleted_at IS NULL LIMIT 50000`, a.Org)
	if err != nil {
		return ConflictResult{}, err
	}
	for prows.Next() {
		var id, name, email, phone, role string
		var m Matter
		if prows.Scan(&id, &name, &email, &phone, &role, &m.ID, &m.Number, &m.Title, &m.Status, &m.Access, &m.SharedWith, &m.CreatedBy, &m.ResponsibleID, &m.OriginatingID) == nil {
			cands = append(cands, Candidate{Source: "party", ID: id, Name: name, Extra: []string{email, phone}, Role: role, Matter: m.ID})
			addMatter(key("party", id), m, role)
		}
	}
	prows.Close()
	// Contacts (not yet clients or parties).
	crows, err := r.pool.Query(ctx, `SELECT id::text, display_name, company, emails::text, phones::text FROM adrient.contacts WHERE org_id=$1 AND trashed_at IS NULL LIMIT 20000`, a.Org)
	if err == nil {
		for crows.Next() {
			var id, name, company, emails, phones string
			if crows.Scan(&id, &name, &company, &emails, &phones) == nil {
				cands = append(cands, Candidate{Source: "contact", ID: id, Name: name, Extra: []string{company, emails, phones}, Role: "Contact"})
			}
		}
		crows.Close()
	}
	hits := RankHits(query, cands)
	res := ConflictResult{Query: query, Hits: []ConflictHit{}}
	for _, h := range hits {
		ch := ConflictHit{Hit: h, Matters: []ConflictMatter{}}
		for _, mr := range appear[key(h.Source, h.ID)] {
			cm := mr.ConflictMatter
			if !mr.allowed {
				cm = ConflictMatter{Role: mr.Role, Restricted: true}
			}
			ch.Matters = append(ch.Matters, cm)
		}
		res.Hits = append(res.Hits, ch)
		if len(res.Hits) >= 50 {
			break
		}
	}
	res.Clear = len(res.Hits) == 0
	return res, nil
}

// RecordConflict saves a conflict check on a matter: a note with the result, and the date it was cleared.
func (r *Repository) RecordConflict(ctx context.Context, a knowkit.Actor, matterID, query string) (ConflictResult, error) {
	if _, err := r.loadMatter(ctx, a, matterID); err != nil {
		return ConflictResult{}, err
	}
	res, err := r.ConflictCheck(ctx, a, query)
	if err != nil {
		return res, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Conflict check for “%s” on %s: ", res.Query, r.today())
	if res.Clear {
		b.WriteString("no matches.")
	} else {
		fmt.Fprintf(&b, "%d match(es).", len(res.Hits))
		for _, h := range res.Hits {
			fmt.Fprintf(&b, "\n- %s (%s)", h.Name, h.Role)
			for _, m := range h.Matters {
				if m.Restricted {
					fmt.Fprintf(&b, " on a restricted matter as %s", m.Role)
				} else {
					fmt.Fprintf(&b, " on %s %s as %s", m.Number, m.Title, m.Role)
				}
			}
		}
	}
	if _, err := r.AddNote(ctx, a, matterID, "conflict", b.String()); err != nil {
		return res, err
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.legal_matters SET conflict_checked_at=now(), conflict_checked_by=$3 WHERE org_id=$1 AND id=$2`, a.Org, matterID, a.User); err != nil {
		return res, err
	}
	r.audit(ctx, "conflict_check", "legal_matter", matterID)
	return res, nil
}

// ---- overview and reports ----------------------------------------------------------------------------------

// Overview is the dashboard.
type Overview struct {
	ByStatus     map[string]int `json:"by_status"`
	OpenMatters  int            `json:"open_matters"`
	Unbilled     int64          `json:"unbilled"`
	UnbilledMins int            `json:"unbilled_minutes"`
	Billed30     int64          `json:"billed_30_days"`
	MinutesMonth int            `json:"minutes_this_month"`
	OverdueCount int            `json:"overdue_deadlines"`
	SoonCount    int            `json:"deadlines_next_7_days"`
	Deadlines    []Deadline     `json:"deadlines"`
	SolWatch     []Matter       `json:"limitation_watch"`
	Recent       []*Matter      `json:"recent_matters"`
	Currency     string         `json:"currency"`
}

// Overview builds the dashboard for the matters the actor may see.
func (r *Repository) Overview(ctx context.Context, a knowkit.Actor) (Overview, error) {
	o := Overview{ByStatus: map[string]int{}}
	s, err := r.GetSettings(ctx, a.Org)
	if err != nil {
		return o, err
	}
	o.Currency = s.Currency
	ms, err := r.Matters(ctx, a, MatterFilter{})
	if err != nil {
		return o, err
	}
	for _, m := range ms {
		o.ByStatus[m.Status]++
		if m.Status == "open" || m.Status == "pending" || m.Status == "on_hold" {
			o.OpenMatters++
		}
		if m.Status != "closed" {
			o.Unbilled += m.Unbilled
			o.UnbilledMins += m.UnbilledMinutes
		}
		if m.SolDate != "" && m.Status != "closed" && m.SolDays != nil && *m.SolDays <= 90 {
			o.SolWatch = append(o.SolWatch, *m)
		}
	}
	sort.Slice(o.SolWatch, func(i, j int) bool { return o.SolWatch[i].SolDate < o.SolWatch[j].SolDate })
	if len(ms) > 6 {
		o.Recent = ms[:6]
	} else {
		o.Recent = ms
	}
	dl, err := r.Deadlines(ctx, a, false)
	if err != nil {
		return o, err
	}
	for _, d := range dl {
		switch d.Urgency {
		case "overdue":
			o.OverdueCount++
		case "soon":
			o.SoonCount++
		}
		if len(o.Deadlines) < 8 {
			o.Deadlines = append(o.Deadlines, d)
		}
	}
	if o.Deadlines == nil {
		o.Deadlines = []Deadline{}
	}
	if o.SolWatch == nil {
		o.SolWatch = []Matter{}
	}
	first := r.today()[:8] + "01"
	ents, err := r.Entries(ctx, a, "", "", first, "", "time")
	if err != nil {
		return o, err
	}
	for _, e := range ents {
		o.MinutesMonth += e.Minutes
	}
	bills, err := r.Bills(ctx, a, "")
	if err != nil {
		return o, err
	}
	cut := AddDays(r.today(), -30)
	for _, b := range bills {
		if b.Status != "void" && b.IssueDate >= cut {
			o.Billed30 += b.Total
		}
	}
	return o, nil
}

// UserHours is one row of the hours report.
type UserHours struct {
	UserID   string `json:"user_id"`
	User     string `json:"user"`
	Minutes  int    `json:"minutes"`
	Billable int    `json:"billable_minutes"`
	Value    int64  `json:"value"`
}

// HoursByUser sums time per person over a period (admins only: it covers everybody).
func (r *Repository) HoursByUser(ctx context.Context, a knowkit.Actor, from, to string) ([]UserHours, error) {
	if !a.Admin {
		return nil, knowkit.ErrForbidden
	}
	if (from != "" && !ValidDate(from)) || (to != "" && !ValidDate(to)) {
		return nil, knowkit.Invalid("from and to must be dates")
	}
	rows, err := r.pool.Query(ctx, `SELECT e.user_id::text, COALESCE(NULLIF(u.display_name,''),u.email,''), sum(e.minutes), sum(e.minutes) FILTER (WHERE e.billable), COALESCE(sum(e.amount) FILTER (WHERE e.billable),0)
		FROM adrient.legal_entries e LEFT JOIN adrient.users u ON u.id=e.user_id WHERE e.org_id=$1 AND e.kind='time'
		AND ($2='' OR e.entry_date >= $2::date) AND ($3='' OR e.entry_date <= $3::date) GROUP BY 1,2 ORDER BY 3 DESC`, a.Org, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserHours{}
	for rows.Next() {
		var h UserHours
		var b *int
		if err := rows.Scan(&h.UserID, &h.User, &h.Minutes, &b, &h.Value); err != nil {
			return nil, err
		}
		if b != nil {
			h.Billable = *b
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
