package talent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"
)

// OfferInput creates an offer letter for an application.
type OfferInput struct {
	JobTitle   string  `json:"job_title"`
	Department string  `json:"department"`
	Salary     float64 `json:"salary"`
	Currency   string  `json:"currency"`
	StartDate  string  `json:"start_date"`
	Body       string  `json:"body"`
	ValidDays  int     `json:"valid_days"`
}

// RenderOffer fills the {{placeholders}} of an offer template.
func RenderOffer(tpl string, vars map[string]string) string {
	if strings.TrimSpace(tpl) == "" {
		tpl = DefaultOfferTemplate
	}
	for k, v := range vars {
		tpl = strings.ReplaceAll(tpl, "{{"+k+"}}", v)
	}
	return tpl
}

// FormatMoney renders an amount with two decimals and its currency code.
func FormatMoney(v float64, cur string) string {
	return fmt.Sprintf("%s %.2f", cur, v)
}

func newToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateOffer drafts an offer letter, prefilled from the job and candidate.
func (r *Repository) CreateOffer(ctx context.Context, a Actor, appID string, in OfferInput) (json.RawMessage, error) {
	f, err := r.leadApp(ctx, a, appID)
	if err != nil {
		return nil, err
	}
	if f.Status != "active" {
		return nil, invalid("this application is %s", f.Status)
	}
	if in.Salary < 0 {
		return nil, invalid("salary cannot be negative")
	}
	in.JobTitle = clean(in.JobTitle)
	if in.JobTitle == "" {
		in.JobTitle = f.Job.Title
	}
	if in.Department == "" {
		in.Department = f.Job.Department
	}
	if in.Currency == "" {
		in.Currency = f.Job.Currency
	}
	var start any
	if in.StartDate != "" {
		t, err := time.Parse("2006-01-02", in.StartDate)
		if err != nil {
			return nil, invalid("start date must be YYYY-MM-DD")
		}
		start = t
	}
	if in.ValidDays <= 0 {
		in.ValidDays = 14
	}
	if in.Body == "" {
		st, err := r.GetSettings(ctx, a.OrgID)
		if err != nil {
			return nil, err
		}
		var name, company, sender string
		_ = r.pool.QueryRow(ctx, `SELECT c.name FROM adrient.talent_candidates c WHERE c.id=$1`, f.CandidateID).Scan(&name)
		_ = r.pool.QueryRow(ctx, `SELECT display_name FROM adrient.orgs WHERE id=$1`, a.OrgID).Scan(&company)
		_ = r.pool.QueryRow(ctx, `SELECT COALESCE(NULLIF(display_name,''),email) FROM adrient.users WHERE id=$1`, a.UserID).Scan(&sender)
		sd := in.StartDate
		if sd == "" {
			sd = "to be agreed"
		}
		in.Body = RenderOffer(st.OfferTemplate, map[string]string{
			"name": name, "job_title": in.JobTitle, "department": in.Department, "salary": FormatMoney(in.Salary, in.Currency),
			"start_date": sd, "company": company, "sender": sender,
		})
	}
	tok, err := newToken()
	if err != nil {
		return nil, err
	}
	var id string
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.talent_offers (org_id, application_id, job_title, department, salary, currency, start_date, body, accept_token, expires_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now() + make_interval(days => $10),$11) RETURNING id::text`,
		a.OrgID, appID, in.JobTitle, in.Department, in.Salary, in.Currency, start, in.Body, tok, in.ValidDays, a.UserID).Scan(&id)
	if err != nil {
		return nil, err
	}
	r.audit(ctx, a, "offer.create", "talent_offer", id)
	return r.row(ctx, `SELECT id::text AS id, job_title, department, salary::float8 AS salary, currency, start_date, status, body, accept_token, expires_at
		FROM adrient.talent_offers WHERE id=$1`, id)
}

// OfferPDF renders the offer letter of an offer the actor may manage.
func (r *Repository) OfferPDF(ctx context.Context, a Actor, offerID string) ([]byte, string, error) {
	var appID, title, body string
	if err := r.pool.QueryRow(ctx, `SELECT application_id::text, job_title, body FROM adrient.talent_offers WHERE org_id=$1 AND id=$2`, a.OrgID, offerID).
		Scan(&appID, &title, &body); err != nil {
		return nil, "", mapNoRows(err)
	}
	if _, err := r.leadApp(ctx, a, appID); err != nil {
		return nil, "", err
	}
	return BuildPDF("Offer of employment: "+title, body), "offer-" + offerID[:8] + ".pdf", nil
}

// SendOffer mails the candidate the e-accept link and marks the offer sent.
func (r *Repository) SendOffer(ctx context.Context, a Actor, offerID, baseURL string) (json.RawMessage, error) {
	var appID, tok, status, title string
	if err := r.pool.QueryRow(ctx, `SELECT application_id::text, accept_token, status, job_title FROM adrient.talent_offers WHERE org_id=$1 AND id=$2`, a.OrgID, offerID).
		Scan(&appID, &tok, &status, &title); err != nil {
		return nil, mapNoRows(err)
	}
	f, err := r.leadApp(ctx, a, appID)
	if err != nil {
		return nil, err
	}
	if status != "draft" && status != "sent" {
		return nil, invalid("this offer is %s", status)
	}
	var name, email string
	_ = r.pool.QueryRow(ctx, `SELECT name, email FROM adrient.talent_candidates WHERE id=$1`, f.CandidateID).Scan(&name, &email)
	link := strings.TrimRight(baseURL, "/") + "/careers/offer/" + tok
	text := fmt.Sprintf("Hello %s,\n\nWe are pleased to send you an offer for %s.\nReview and respond here: %s\n", name, title, link)
	h := fmt.Sprintf(`<p>Hello %s,</p><p>We are pleased to send you an offer for <b>%s</b>.</p><p><a href="%s">Review and respond to your offer</a></p>`,
		html.EscapeString(name), html.EscapeString(title), html.EscapeString(link))
	// Without a mail transport the offer is still marked sent; the recruiter
	// copies the e-accept link (mail_sent=false tells the UI to say so).
	mailed := false
	if r.Deps.Mail != nil {
		if err := r.Deps.Mail(ctx, []string{email}, "Your offer: "+title, h, text); err != nil {
			return nil, err
		}
		mailed = true
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_offers SET status='sent', sent_at=now() WHERE id=$1`, offerID); err != nil {
		return nil, err
	}
	r.audit(ctx, a, "offer.send", "talent_offer", offerID)
	return r.row(ctx, `SELECT id::text AS id, status, sent_at, accept_token, $2::boolean AS mail_sent, $3::text AS link FROM adrient.talent_offers WHERE id=$1`, offerID, mailed, link)
}

// WithdrawOffer withdraws a draft or sent offer.
func (r *Repository) WithdrawOffer(ctx context.Context, a Actor, offerID string) error {
	var appID, status string
	if err := r.pool.QueryRow(ctx, `SELECT application_id::text, status FROM adrient.talent_offers WHERE org_id=$1 AND id=$2`, a.OrgID, offerID).Scan(&appID, &status); err != nil {
		return mapNoRows(err)
	}
	if _, err := r.leadApp(ctx, a, appID); err != nil {
		return err
	}
	if status == "accepted" || status == "declined" {
		return invalid("this offer was already answered")
	}
	_, err := r.pool.Exec(ctx, `UPDATE adrient.talent_offers SET status='withdrawn' WHERE id=$1`, offerID)
	return err
}

// PublicOffer is what the candidate sees at the e-accept link.
type PublicOffer struct {
	Candidate string     `json:"candidate"`
	Company   string     `json:"company"`
	JobTitle  string     `json:"job_title"`
	Body      string     `json:"body"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
	Expired   bool       `json:"expired"`
}

// GetPublicOffer loads an offer by its unguessable token.
func (r *Repository) GetPublicOffer(ctx context.Context, token string) (PublicOffer, string, error) {
	var p PublicOffer
	var org string
	err := r.pool.QueryRow(ctx, `SELECT o.org_id::text, c.name, org.display_name, o.job_title, o.body, o.status, o.expires_at
		FROM adrient.talent_offers o JOIN adrient.talent_applications ap ON ap.id=o.application_id
		JOIN adrient.talent_candidates c ON c.id=ap.candidate_id JOIN adrient.orgs org ON org.id=o.org_id
		WHERE o.accept_token=$1 AND o.status <> 'draft'`, token).Scan(&org, &p.Candidate, &p.Company, &p.JobTitle, &p.Body, &p.Status, &p.ExpiresAt)
	if err != nil {
		return p, "", mapNoRows(err)
	}
	p.Expired = p.Status == "sent" && p.ExpiresAt != nil && p.ExpiresAt.Before(r.now())
	return p, org, nil
}

// RespondOffer records the candidate's accept or decline. Accepting needs a typed name.
func (r *Repository) RespondOffer(ctx context.Context, token string, accept bool, name string) error {
	p, org, err := r.GetPublicOffer(ctx, token)
	if err != nil {
		return err
	}
	if p.Status != "sent" {
		return invalid("this offer is %s", p.Status)
	}
	if p.Expired {
		return invalid("this offer has expired")
	}
	name = clean(name)
	if accept && (len(name) < 2 || len(name) > 120) {
		return invalid("type your full name to accept")
	}
	st := "declined"
	if accept {
		st = "accepted"
	}
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.talent_offers SET status=$2, responded_at=now(), responded_name=$3 WHERE accept_token=$1 AND status='sent'`, token, st, name); err != nil {
		return err
	}
	// Tell the recruiter and hiring manager.
	if r.Deps.Mail != nil {
		rows, err := r.pool.Query(ctx, `SELECT DISTINCT u.email FROM adrient.talent_offers o JOIN adrient.talent_applications ap ON ap.id=o.application_id
			JOIN adrient.talent_jobs j ON j.id=ap.job_id JOIN adrient.users u ON u.id IN (j.recruiter_id, j.hiring_manager_id, o.created_by)
			WHERE o.accept_token=$1 AND u.org_id=$2`, token, org)
		if err == nil {
			var to []string
			for rows.Next() {
				var e string
				if rows.Scan(&e) == nil {
					to = append(to, e)
				}
			}
			rows.Close()
			if len(to) > 0 {
				msg := fmt.Sprintf("%s has %s the offer for %s.", p.Candidate, st, p.JobTitle)
				_ = r.Deps.Mail(ctx, to, "Offer "+st+": "+p.Candidate, "<p>"+html.EscapeString(msg)+"</p>", msg)
			}
		}
	}
	return nil
}

// PublicOfferPDF renders the letter for the candidate.
func (r *Repository) PublicOfferPDF(ctx context.Context, token string) ([]byte, error) {
	p, _, err := r.GetPublicOffer(ctx, token)
	if err != nil {
		return nil, err
	}
	return BuildPDF("Offer of employment: "+p.JobTitle, p.Body), nil
}

// NewHire is the data HR core needs to create an employee from a hired candidate.
type NewHire struct {
	ApplicationID string  `json:"application_id"`
	CandidateID   string  `json:"candidate_id"`
	Name          string  `json:"name"`
	Email         string  `json:"email"`
	Phone         string  `json:"phone"`
	JobTitle      string  `json:"job_title"`
	Department    string  `json:"department"`
	StartDate     string  `json:"start_date"`
	SalaryOffer   float64 `json:"salary_offer"`
	Currency      string  `json:"currency"`
	ManagerID     string  `json:"manager_id"`
}

// ConvertToNewHire returns the new-hire record for a hired application or one
// whose offer was accepted. HR only. The integration step feeds this to HR core.
func (r *Repository) ConvertToNewHire(ctx context.Context, a Actor, appID string) (NewHire, error) {
	var h NewHire
	if !a.IsHR {
		return h, ErrForbidden
	}
	var start *time.Time
	err := r.pool.QueryRow(ctx, `SELECT ap.id::text, c.id::text, c.name, c.email, c.phone,
		COALESCE(o.job_title, j.title), COALESCE(o.department, j.department), o.start_date, COALESCE(o.salary,0)::float8,
		COALESCE(o.currency, j.currency), COALESCE(j.hiring_manager_id::text,''), ap.status, COALESCE(o.status,'')
		FROM adrient.talent_applications ap JOIN adrient.talent_candidates c ON c.id=ap.candidate_id JOIN adrient.talent_jobs j ON j.id=ap.job_id
		LEFT JOIN LATERAL (SELECT * FROM adrient.talent_offers x WHERE x.application_id=ap.id AND x.status='accepted' ORDER BY x.responded_at DESC LIMIT 1) o ON true
		WHERE ap.org_id=$1 AND ap.id=$2`, a.OrgID, appID).Scan(&h.ApplicationID, &h.CandidateID, &h.Name, &h.Email, &h.Phone,
		&h.JobTitle, &h.Department, &start, &h.SalaryOffer, &h.Currency, &h.ManagerID, new(string), new(string))
	if err != nil {
		return h, mapNoRows(err)
	}
	var status, offer string
	_ = r.pool.QueryRow(ctx, `SELECT ap.status, COALESCE((SELECT x.status FROM adrient.talent_offers x WHERE x.application_id=ap.id AND x.status='accepted' LIMIT 1),'')
		FROM adrient.talent_applications ap WHERE ap.id=$1`, appID).Scan(&status, &offer)
	if status != "hired" && offer != "accepted" {
		return h, invalid("the candidate is not hired and has no accepted offer")
	}
	if start != nil {
		h.StartDate = start.Format("2006-01-02")
	}
	return h, nil
}

// ---------- reports ----------

// Reports are the recruitment analytics (HR only).
type Reports struct {
	TimeToHire []TimeToHire `json:"time_to_hire"`
	AvgDays    float64      `json:"avg_days_to_hire"`
	Sources    []SourceRow  `json:"sources"`
	Funnel     []FunnelRow  `json:"funnel"`
}

// TimeToHire is the hiring speed of one job.
type TimeToHire struct {
	JobID   string  `json:"job_id"`
	Title   string  `json:"title"`
	Hired   int     `json:"hired"`
	AvgDays float64 `json:"avg_days"`
}

// SourceRow measures one source's quality.
type SourceRow struct {
	Source       string  `json:"source"`
	Applications int     `json:"applications"`
	Interviewed  int     `json:"interviewed"`
	Hired        int     `json:"hired"`
	HireRate     float64 `json:"hire_rate"`
}

// FunnelRow counts applications that ever reached a stage.
type FunnelRow struct {
	Stage string `json:"stage"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Reports computes the analytics, optionally for one job.
func (r *Repository) Reports(ctx context.Context, a Actor, jobID string) (Reports, error) {
	out := Reports{TimeToHire: []TimeToHire{}, Sources: []SourceRow{}, Funnel: []FunnelRow{}}
	if !a.IsHR {
		return out, ErrForbidden
	}
	st, err := r.GetSettings(ctx, a.OrgID)
	if err != nil {
		return out, err
	}
	rows, err := r.pool.Query(ctx, `SELECT j.id::text, j.title, count(*), round(avg(extract(epoch FROM ap.closed_at - ap.applied_at)/86400)::numeric,1)::float8
		FROM adrient.talent_applications ap JOIN adrient.talent_jobs j ON j.id=ap.job_id
		WHERE ap.org_id=$1 AND ap.status='hired' AND ($2='' OR ap.job_id::text=$2) GROUP BY j.id, j.title ORDER BY 4`, a.OrgID, jobID)
	if err != nil {
		return out, err
	}
	var totalDays float64
	var totalHired int
	for rows.Next() {
		var t TimeToHire
		if err := rows.Scan(&t.JobID, &t.Title, &t.Hired, &t.AvgDays); err != nil {
			rows.Close()
			return out, err
		}
		totalDays += t.AvgDays * float64(t.Hired)
		totalHired += t.Hired
		out.TimeToHire = append(out.TimeToHire, t)
	}
	rows.Close()
	if totalHired > 0 {
		out.AvgDays = float64(int(totalDays/float64(totalHired)*10+0.5)) / 10
	}
	srows, err := r.pool.Query(ctx, `SELECT c.source, count(*),
		count(*) FILTER (WHERE EXISTS(SELECT 1 FROM adrient.talent_interviews i WHERE i.application_id=ap.id)),
		count(*) FILTER (WHERE ap.status='hired')
		FROM adrient.talent_applications ap JOIN adrient.talent_candidates c ON c.id=ap.candidate_id
		WHERE ap.org_id=$1 AND ($2='' OR ap.job_id::text=$2) GROUP BY c.source ORDER BY 2 DESC`, a.OrgID, jobID)
	if err != nil {
		return out, err
	}
	for srows.Next() {
		var s SourceRow
		if err := srows.Scan(&s.Source, &s.Applications, &s.Interviewed, &s.Hired); err != nil {
			srows.Close()
			return out, err
		}
		if s.Applications > 0 {
			s.HireRate = float64(int(float64(s.Hired)/float64(s.Applications)*1000+0.5)) / 10
		}
		out.Sources = append(out.Sources, s)
	}
	srows.Close()
	reached := map[string]int{}
	frows, err := r.pool.Query(ctx, `SELECT h.to_stage, count(DISTINCT h.application_id) FROM adrient.talent_stage_history h
		JOIN adrient.talent_applications ap ON ap.id=h.application_id WHERE h.org_id=$1 AND ($2='' OR ap.job_id::text=$2) GROUP BY h.to_stage`, a.OrgID, jobID)
	if err != nil {
		return out, err
	}
	for frows.Next() {
		var s string
		var n int
		if err := frows.Scan(&s, &n); err != nil {
			frows.Close()
			return out, err
		}
		reached[s] = n
	}
	frows.Close()
	for _, s := range st.Stages {
		out.Funnel = append(out.Funnel, FunnelRow{Stage: s.ID, Name: s.Name, Count: reached[s.ID]})
	}
	out.Funnel = append(out.Funnel, FunnelRow{Stage: StageHired, Name: "Hired", Count: reached[StageHired]})
	return out, nil
}

// BuildPDF renders a simple one-column text document as a PDF 1.4 file
// (A4, Helvetica). Characters outside Latin-1 are replaced with '?'.
func BuildPDF(title, body string) []byte {
	const (
		width    = 595
		height   = 842
		margin   = 56
		lineH    = 15
		wrapCols = 88
	)
	lines := []string{}
	for _, para := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(para) == "" {
			lines = append(lines, "")
			continue
		}
		cur := ""
		for _, w := range strings.Fields(para) {
			if cur != "" && len(cur)+1+len(w) > wrapCols {
				lines = append(lines, cur)
				cur = w
			} else if cur == "" {
				cur = w
			} else {
				cur += " " + w
			}
		}
		lines = append(lines, cur)
	}
	perPage := (height - 2*margin - 40) / lineH
	var pages [][]string
	for i := 0; i < len(lines); i += perPage {
		end := i + perPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, lines[i:end])
	}
	if len(pages) == 0 {
		pages = [][]string{{}}
	}
	esc := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			switch {
			case r == '(' || r == ')' || r == '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case r < 32 || r > 255:
				b.WriteByte('?')
			case r > 126:
				fmt.Fprintf(&b, "\\%03o", r)
			default:
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	var objs []string
	// 1 catalog, 2 pages, 3 font, 4 bold font, then page+content pairs.
	n := len(pages)
	kids := make([]string, n)
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 5+i*2)
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	objs = append(objs, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	for i, pl := range pages {
		var c bytes.Buffer
		y := height - margin
		if i == 0 {
			fmt.Fprintf(&c, "BT /F2 15 Tf %d %d Td (%s) Tj ET\n", margin, y, esc(title))
			y -= 40
		}
		c.WriteString("BT /F1 11 Tf\n")
		fmt.Fprintf(&c, "%d %d Td %d TL\n", margin, y, lineH)
		for _, l := range pl {
			fmt.Fprintf(&c, "(%s) Tj T*\n", esc(l))
		}
		c.WriteString("ET\n")
		fmt.Fprintf(&c, "BT /F1 8 Tf %d 30 Td (Page %d of %d) Tj ET\n", margin, i+1, n)
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>", width, height, 6+i*2))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", c.Len(), c.String()))
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return out.Bytes()
}

var _ = errors.New
