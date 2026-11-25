package crm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"
)

// LeadInput is what lead capture (public form, Forms app, import) hands to CreateLead.
type LeadInput struct {
	Name, Email, Phone, Company, Message, Source string
	Custom                                       map[string]any
	OwnerIDs                                     []string // round-robin pool; empty = CreatedBy
	CreatedBy                                    string   // user id that owns the new Contact when none exists
	RRKey                                        string
	FormEventKey                                 string // unique id of the submission (idempotency of form_submitted)
	FormID                                       string
	FormName                                     string
}

// LeadResult reports what CreateLead did.
type LeadResult struct {
	PersonID  string `json:"person_id"`
	ContactID string `json:"contact_id"`
	Created   bool   `json:"created"`
}

// ValidEmail reports whether s is a plain address.
func ValidEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == strings.TrimSpace(s) && strings.Contains(a.Address, "@")
}

// CreateLead adds a lead, or attaches to the existing contact with the same
// e-mail address (no duplicate), then runs scoring and automation.
func (r *Repository) CreateLead(ctx context.Context, org string, in LeadInput) (LeadResult, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = NormEmail(in.Email)
	in.Phone = strings.TrimSpace(in.Phone)
	if in.Name == "" && in.Email == "" {
		return LeadResult{}, invalid("a name or an email address is required")
	}
	if in.Email != "" && !ValidEmail(in.Email) {
		return LeadResult{}, invalid("invalid email address")
	}
	if in.Name == "" {
		in.Name = in.Email
	}
	if len(in.Name) > 200 || len(in.Message) > 5000 || len(in.Phone) > 50 || len(in.Company) > 200 {
		return LeadResult{}, invalid("a field is too long")
	}
	creator := in.CreatedBy
	if creator == "" {
		_ = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.users WHERE org_id=$1 ORDER BY created_at LIMIT 1`, org).Scan(&creator)
	}
	if creator == "" {
		return LeadResult{}, invalid("no user available to own the lead")
	}
	custom, err := r.checkCustom(ctx, org, "person", in.Custom, false)
	if err != nil {
		return LeadResult{}, err
	}
	var res LeadResult
	if in.Email != "" {
		err = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.contacts c WHERE org_id=$1 AND trashed_at IS NULL AND
			EXISTS (SELECT 1 FROM jsonb_array_elements_text(c.emails) e WHERE lower(e)=$2) ORDER BY created_at LIMIT 1`, org, in.Email).Scan(&res.ContactID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return res, err
		}
	}
	if res.ContactID == "" {
		emails, _ := json.Marshal(nonEmpty(in.Email))
		phones, _ := json.Marshal(nonEmpty(in.Phone))
		first, last, _ := strings.Cut(in.Name, " ")
		if err := r.pool.QueryRow(ctx, `INSERT INTO adrient.contacts (org_id,owner_id,display_name,first_name,last_name,company,emails,phones)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, org, creator, in.Name, first, last, in.Company, emails, phones).Scan(&res.ContactID); err != nil {
			return res, err
		}
	}
	var companyID any
	if c := strings.TrimSpace(in.Company); c != "" {
		var cid string
		err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.crm_companies WHERE org_id=$1 AND lower(name)=lower($2) LIMIT 1`, org, c).Scan(&cid)
		if errors.Is(err, pgx.ErrNoRows) {
			err = r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_companies (org_id,name,created_by) VALUES ($1,$2,$3) RETURNING id::text`, org, c, creator).Scan(&cid)
		}
		if err != nil {
			return res, err
		}
		companyID = cid
	}
	owner := creator
	if len(in.OwnerIDs) > 0 {
		key := in.RRKey
		if key == "" {
			key = "capture"
		}
		idx, err := r.nextCounter(ctx, org, key)
		if err != nil {
			return res, err
		}
		owner = PickRoundRobin(in.OwnerIDs, idx)
	}
	cj, _ := json.Marshal(nonNilMap(custom))
	src := strings.TrimSpace(in.Source)
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_people (org_id,contact_id,company_id,status,source,notes,owner_id,created_by,custom)
		VALUES ($1,$2,$3,'lead',$4,$5,$6,$7,$8) ON CONFLICT (org_id,contact_id) DO NOTHING RETURNING id::text`,
		org, res.ContactID, companyID, src, in.Message, owner, creator, cj).Scan(&res.PersonID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.crm_people WHERE org_id=$1 AND contact_id=$2`, org, res.ContactID).Scan(&res.PersonID)
		if err == nil && len(custom) > 0 {
			_, err = r.pool.Exec(ctx, `UPDATE adrient.crm_people SET custom=custom || $3 WHERE org_id=$1 AND id=$2`, org, res.PersonID, cj)
		}
	} else if err == nil {
		res.Created = true
	}
	if err != nil {
		return res, err
	}
	if in.FormEventKey != "" {
		note := "Submitted " + firstNonEmpty(in.FormName, "a form")
		if in.Message != "" {
			note += ": " + truncate(in.Message, 300)
		}
		_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.crm_activities (org_id,kind,summary,contact_id,company_id) VALUES ($1,'note',$2,$3,$4)`, org, note, res.ContactID, companyID)
	}
	if err := r.RescorePerson(ctx, org, res.PersonID); err != nil {
		slog.Warn("crm rescore", "err", err)
	}
	if res.Created {
		r.fire(ctx, Event{Org: org, Trigger: TrigLeadCreated, PersonID: res.PersonID, Source: src, Key: "lead:" + res.PersonID})
	}
	if in.FormEventKey != "" {
		r.fire(ctx, Event{Org: org, Trigger: TrigFormSubmitted, PersonID: res.PersonID, FormID: in.FormID, Source: src, Key: "form:" + in.FormEventKey})
	}
	return res, nil
}

func nonEmpty(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	return []string{s}
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}

// ---- public lead forms ----

// FormField is one input on a public lead form.
type FormField struct {
	Key      string `json:"key"` // name | email | phone | company | message | custom:<key>
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// LeadForm is an embeddable web-to-lead form.
type LeadForm struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Token    string      `json:"token"`
	Enabled  bool        `json:"enabled"`
	Fields   []FormField `json:"fields"`
	Source   string      `json:"source"`
	OwnerIDs []string    `json:"owner_ids"`
	Thanks   string      `json:"thanks"`
}

var baseFormKeys = map[string]bool{"name": true, "email": true, "phone": true, "company": true, "message": true}

func (f *LeadForm) validate() error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return invalid("form name is required")
	}
	if len(f.Fields) == 0 || len(f.Fields) > 20 {
		return invalid("a form needs between 1 and 20 fields")
	}
	hasContact := false
	for i := range f.Fields {
		k := f.Fields[i].Key
		if !baseFormKeys[k] && !strings.HasPrefix(k, "custom:") {
			return invalid("unknown field %q", k)
		}
		if k == "name" || k == "email" {
			hasContact = true
		}
		if strings.TrimSpace(f.Fields[i].Label) == "" {
			f.Fields[i].Label = strings.Title(strings.TrimPrefix(k, "custom:")) //nolint:staticcheck
		}
	}
	if !hasContact {
		return invalid("a form needs a name or email field")
	}
	if f.Source == "" {
		f.Source = "web form"
	}
	if f.Thanks == "" {
		f.Thanks = "Thanks, we will be in touch."
	}
	if f.OwnerIDs == nil {
		f.OwnerIDs = []string{}
	}
	return nil
}

func newToken() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Repository) ListLeadForms(ctx context.Context, org string) ([]LeadForm, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, token, enabled, fields, source, owner_ids, thanks FROM adrient.crm_lead_forms WHERE org_id=$1 ORDER BY created_at`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeadForm{}
	for rows.Next() {
		var f LeadForm
		var fj, oj []byte
		if err := rows.Scan(&f.ID, &f.Name, &f.Token, &f.Enabled, &fj, &f.Source, &oj, &f.Thanks); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(fj, &f.Fields)
		_ = json.Unmarshal(oj, &f.OwnerIDs)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *Repository) SaveLeadForm(ctx context.Context, org, user, id string, f LeadForm) (LeadForm, error) {
	if err := f.validate(); err != nil {
		return f, err
	}
	fj, _ := json.Marshal(f.Fields)
	oj, _ := json.Marshal(f.OwnerIDs)
	if id == "" {
		f.Token = newToken()
		f.Enabled = true
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_lead_forms (org_id,name,token,enabled,fields,source,owner_ids,thanks,created_by)
			VALUES ($1,$2,$3,true,$4,$5,$6,$7,$8) RETURNING id::text`, org, f.Name, f.Token, fj, f.Source, oj, f.Thanks, user).Scan(&f.ID)
		return f, err
	}
	err := r.pool.QueryRow(ctx, `UPDATE adrient.crm_lead_forms SET name=$3,enabled=$4,fields=$5,source=$6,owner_ids=$7,thanks=$8
		WHERE org_id=$1 AND id=$2 RETURNING id::text, token`, org, id, f.Name, f.Enabled, fj, f.Source, oj, f.Thanks).Scan(&f.ID, &f.Token)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

func (r *Repository) DeleteLeadForm(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_lead_forms WHERE org_id=$1 AND id=$2`, org, id)
}

// LeadFormByToken loads an enabled form plus its org and creator for public submissions.
func (r *Repository) LeadFormByToken(ctx context.Context, token string) (f LeadForm, org, creator string, err error) {
	var fj, oj []byte
	err = r.pool.QueryRow(ctx, `SELECT id::text, name, token, enabled, fields, source, owner_ids, thanks, org_id::text, created_by::text
		FROM adrient.crm_lead_forms WHERE token=$1 AND enabled`, token).Scan(&f.ID, &f.Name, &f.Token, &f.Enabled, &fj, &f.Source, &oj, &f.Thanks, &org, &creator)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	_ = json.Unmarshal(fj, &f.Fields)
	_ = json.Unmarshal(oj, &f.OwnerIDs)
	return
}

// SubmitLeadForm validates a public submission against the form's fields and creates the lead.
// values are the raw submitted strings by field key; submissionID makes it idempotent.
func (r *Repository) SubmitLeadForm(ctx context.Context, token string, values map[string]string, submissionID string) (LeadResult, string, error) {
	f, org, creator, err := r.LeadFormByToken(ctx, token)
	if err != nil {
		return LeadResult{}, "", err
	}
	in := LeadInput{Source: f.Source, OwnerIDs: f.OwnerIDs, CreatedBy: creator, RRKey: "lf:" + f.ID,
		FormEventKey: "lf:" + f.ID + ":" + submissionID, FormID: f.ID, FormName: f.Name, Custom: map[string]any{}}
	for _, fld := range f.Fields {
		v := strings.TrimSpace(values[fld.Key])
		if fld.Required && v == "" {
			return LeadResult{}, "", invalid("%s is required", fld.Label)
		}
		switch {
		case fld.Key == "name":
			in.Name = v
		case fld.Key == "email":
			in.Email = v
		case fld.Key == "phone":
			in.Phone = v
		case fld.Key == "company":
			in.Company = v
		case fld.Key == "message":
			in.Message = v
		case strings.HasPrefix(fld.Key, "custom:") && v != "":
			in.Custom[strings.TrimPrefix(fld.Key, "custom:")] = coerceCustom(v)
		}
	}
	res, err := r.CreateLead(ctx, org, in)
	return res, f.Thanks, err
}

// coerceCustom turns a submitted string into a value a custom field accepts
// (multi-select values arrive comma separated).
func coerceCustom(v string) any {
	if strings.Contains(v, ",") {
		parts := []any{}
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				parts = append(parts, p)
			}
		}
		return parts
	}
	return v
}

// ---- Forms app link ----

// FormLink maps a Forms-app form to lead fields.
type FormLink struct {
	ID       string            `json:"id"`
	FormID   string            `json:"form_id"`
	Mapping  map[string]string `json:"mapping"` // question id -> name|email|phone|company|message|custom:<key>
	Source   string            `json:"source"`
	OwnerIDs []string          `json:"owner_ids"`
	Enabled  bool              `json:"enabled"`
}

func (r *Repository) ListFormLinks(ctx context.Context, org string) ([]FormLink, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, form_id, mapping, source, owner_ids, enabled FROM adrient.crm_form_links WHERE org_id=$1 ORDER BY form_id`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FormLink{}
	for rows.Next() {
		var l FormLink
		var m, o []byte
		if err := rows.Scan(&l.ID, &l.FormID, &m, &l.Source, &o, &l.Enabled); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(m, &l.Mapping)
		_ = json.Unmarshal(o, &l.OwnerIDs)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) SaveFormLink(ctx context.Context, org, user string, l FormLink) (FormLink, error) {
	l.FormID = strings.TrimSpace(l.FormID)
	if l.FormID == "" || len(l.Mapping) == 0 {
		return l, invalid("a form and at least one mapped question are required")
	}
	hasContact := false
	for q, f := range l.Mapping {
		if q == "" || (!baseFormKeys[f] && !strings.HasPrefix(f, "custom:")) {
			return l, invalid("unknown lead field %q", f)
		}
		if f == "name" || f == "email" {
			hasContact = true
		}
	}
	if !hasContact {
		return l, invalid("map a question to name or email")
	}
	if l.Source == "" {
		l.Source = "forms"
	}
	if l.OwnerIDs == nil {
		l.OwnerIDs = []string{}
	}
	m, _ := json.Marshal(l.Mapping)
	o, _ := json.Marshal(l.OwnerIDs)
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_form_links (org_id,form_id,mapping,source,owner_ids,enabled,created_by) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (org_id,form_id) DO UPDATE SET mapping=EXCLUDED.mapping, source=EXCLUDED.source, owner_ids=EXCLUDED.owner_ids, enabled=EXCLUDED.enabled
		RETURNING id::text`, org, l.FormID, m, l.Source, o, l.Enabled, user).Scan(&l.ID)
	return l, err
}

func (r *Repository) DeleteFormLink(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_form_links WHERE org_id=$1 AND id=$2`, org, id)
}

// FromFormsResponse is called by the Forms app after a response is stored. It
// is a no-op unless the form is linked. answers are the stored answers by question id.
func (r *Repository) FromFormsResponse(ctx context.Context, org, formID, formTitle, responseID, respondentEmail string, answers map[string]any) error {
	var link FormLink
	var m, o []byte
	var creator *string
	err := r.pool.QueryRow(ctx, `SELECT id::text, form_id, mapping, source, owner_ids, enabled, created_by::text FROM adrient.crm_form_links
		WHERE org_id=$1 AND form_id=$2 AND enabled`, org, formID).Scan(&link.ID, &link.FormID, &m, &link.Source, &o, &link.Enabled, &creator)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = json.Unmarshal(m, &link.Mapping)
	_ = json.Unmarshal(o, &link.OwnerIDs)
	in := LeadInput{Source: link.Source, OwnerIDs: link.OwnerIDs, RRKey: "fl:" + link.ID, FormID: formID, FormName: formTitle,
		FormEventKey: "forms:" + responseID, Custom: map[string]any{}}
	if creator != nil {
		in.CreatedBy = *creator
	}
	for q, field := range link.Mapping {
		v, ok := answers[q]
		if !ok {
			continue
		}
		s := answerString(v)
		switch {
		case field == "name":
			in.Name = s
		case field == "email":
			in.Email = s
		case field == "phone":
			in.Phone = s
		case field == "company":
			in.Company = s
		case field == "message":
			in.Message = s
		case strings.HasPrefix(field, "custom:") && s != "":
			in.Custom[strings.TrimPrefix(field, "custom:")] = coerceCustom(s)
		}
	}
	if in.Email == "" && ValidEmail(respondentEmail) {
		in.Email = respondentEmail
	}
	if _, err := r.CreateLead(ctx, org, in); err != nil {
		return fmt.Errorf("form %s response %s: %w", formID, responseID, err)
	}
	return nil
}

func answerString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, fmt.Sprint(e))
		}
		return strings.Join(parts, ", ")
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// ---- email templates ----

type EmailTemplate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (r *Repository) ListTemplates(ctx context.Context, org string) ([]EmailTemplate, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, subject, body FROM adrient.crm_email_templates WHERE org_id=$1 ORDER BY lower(name)`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EmailTemplate{}
	for rows.Next() {
		var t EmailTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.Body); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) SaveTemplate(ctx context.Context, org, id string, t EmailTemplate) (EmailTemplate, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || strings.TrimSpace(t.Subject) == "" {
		return t, invalid("a template needs a name and a subject")
	}
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_email_templates (org_id,name,subject,body) VALUES ($1,$2,$3,$4) RETURNING id::text`, org, t.Name, t.Subject, t.Body).Scan(&t.ID)
		return t, err
	}
	if err := r.del(ctx, `UPDATE adrient.crm_email_templates SET name=$3,subject=$4,body=$5 WHERE org_id=$1 AND id=$2`, org, id, t.Name, t.Subject, t.Body); err != nil {
		return t, err
	}
	t.ID = id
	return t, nil
}

func (r *Repository) DeleteTemplate(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_email_templates WHERE org_id=$1 AND id=$2`, org, id)
}
