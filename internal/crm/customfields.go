package crm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FieldEntities are the record kinds that can carry custom fields. Leads and
// contacts are the same CRM person; both kinds are stored on crm_people.
var FieldEntities = map[string]bool{"lead": true, "contact": true, "company": true, "deal": true}

// FieldTypes are the supported custom field types.
var FieldTypes = map[string]bool{"text": true, "number": true, "date": true, "select": true, "multiselect": true, "url": true, "user": true}

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// FieldDef is one custom field definition.
type FieldDef struct {
	ID       string   `json:"id"`
	Entity   string   `json:"entity"`
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Options  []string `json:"options"`
	Required bool     `json:"required"`
	Position int      `json:"position"`
}

// Validate checks a definition.
func (d *FieldDef) Validate() error {
	d.Key = strings.TrimSpace(strings.ToLower(d.Key))
	d.Label = strings.TrimSpace(d.Label)
	if d.Type == "" {
		d.Type = "text"
	}
	switch {
	case !FieldEntities[d.Entity]:
		return invalid("unknown entity %q", d.Entity)
	case !FieldTypes[d.Type]:
		return invalid("unknown field type %q", d.Type)
	case !keyRe.MatchString(d.Key):
		return invalid("key must be lower-case letters, digits and underscores, starting with a letter")
	case d.Label == "":
		return invalid("label is required")
	}
	if d.Type == "select" || d.Type == "multiselect" {
		clean := []string{}
		seen := map[string]bool{}
		for _, o := range d.Options {
			o = strings.TrimSpace(o)
			if o != "" && !seen[o] {
				seen[o] = true
				clean = append(clean, o)
			}
		}
		if len(clean) == 0 {
			return invalid("a select field needs at least one option")
		}
		d.Options = clean
	} else {
		d.Options = []string{}
	}
	return nil
}

// ValidateCustom checks and normalises custom values against defs. Unknown
// keys are rejected, empty values are dropped. When checkRequired is set,
// required fields must be present. userOK reports whether a user id exists in
// the org (type "user"); nil skips that check.
func ValidateCustom(defs []FieldDef, vals map[string]any, checkRequired bool, userOK func(string) bool) (map[string]any, error) {
	byKey := map[string]FieldDef{}
	for _, d := range defs {
		byKey[d.Key] = d
	}
	out := map[string]any{}
	for k, v := range vals {
		d, ok := byKey[k]
		if !ok {
			return nil, invalid("unknown custom field %q", k)
		}
		nv, err := normaliseValue(d, v, userOK)
		if err != nil {
			return nil, invalid("%s: %v", d.Label, err)
		}
		if nv != nil {
			out[k] = nv
		}
	}
	if checkRequired {
		for _, d := range defs {
			if d.Required {
				if _, ok := out[d.Key]; !ok {
					return nil, invalid("%s is required", d.Label)
				}
			}
		}
	}
	return out, nil
}

func normaliseValue(d FieldDef, v any, userOK func(string) bool) (any, error) {
	if v == nil {
		return nil, nil
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return nil, nil
	}
	if arr, ok := v.([]any); ok && len(arr) == 0 {
		return nil, nil
	}
	switch d.Type {
	case "text":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be text")
		}
		if len(s) > 2000 {
			return nil, fmt.Errorf("too long")
		}
		return s, nil
	case "number":
		switch t := v.(type) {
		case float64:
			return t, nil
		case int:
			return float64(t), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(t, ",", "")), 64)
			if err != nil {
				return nil, fmt.Errorf("must be a number")
			}
			return f, nil
		}
		return nil, fmt.Errorf("must be a number")
	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be a date")
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, fmt.Errorf("must be a date (YYYY-MM-DD)")
		}
		return s, nil
	case "select":
		s, ok := v.(string)
		if !ok || !contains(d.Options, s) {
			return nil, fmt.Errorf("must be one of the listed options")
		}
		return s, nil
	case "multiselect":
		arr, ok := v.([]any)
		if !ok {
			if ss, ok2 := v.([]string); ok2 {
				for _, s := range ss {
					arr = append(arr, s)
				}
			} else {
				return nil, fmt.Errorf("must be a list")
			}
		}
		res := []string{}
		for _, e := range arr {
			s, ok := e.(string)
			if !ok || !contains(d.Options, s) {
				return nil, fmt.Errorf("must only contain listed options")
			}
			if !contains(res, s) {
				res = append(res, s)
			}
		}
		sort.Strings(res)
		return res, nil
	case "url":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be a URL")
		}
		u, err := url.Parse(strings.TrimSpace(s))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("must be an http(s) URL")
		}
		return u.String(), nil
	case "user":
		s, ok := v.(string)
		if !ok || !uuidRe.MatchString(s) {
			return nil, fmt.Errorf("must be a user")
		}
		if userOK != nil && !userOK(s) {
			return nil, fmt.Errorf("user not found")
		}
		return s, nil
	}
	return nil, fmt.Errorf("unsupported type")
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// ---- repository ----

// ListFieldDefs returns the org's custom field definitions.
func (r *Repository) ListFieldDefs(ctx context.Context, org string) ([]FieldDef, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, entity, key, label, type, options, required, position
		FROM adrient.crm_field_defs WHERE org_id=$1 ORDER BY entity, position, label`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FieldDef{}
	for rows.Next() {
		var d FieldDef
		var opts []byte
		if err := rows.Scan(&d.ID, &d.Entity, &d.Key, &d.Label, &d.Type, &opts, &d.Required, &d.Position); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(opts, &d.Options)
		if d.Options == nil {
			d.Options = []string{}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// defsFor returns the definitions that apply to a record kind ("person" = lead+contact).
func (r *Repository) defsFor(ctx context.Context, org, kind string) ([]FieldDef, error) {
	all, err := r.ListFieldDefs(ctx, org)
	if err != nil {
		return nil, err
	}
	var out []FieldDef
	for _, d := range all {
		if d.Entity == kind || (kind == "person" && (d.Entity == "lead" || d.Entity == "contact")) {
			out = append(out, d)
		}
	}
	return out, nil
}

// checkCustom validates custom values for a record kind (person|company|deal).
func (r *Repository) checkCustom(ctx context.Context, org, kind string, vals map[string]any, create bool) (map[string]any, error) {
	if vals == nil {
		return nil, nil
	}
	defs, err := r.defsFor(ctx, org, kind)
	if err != nil {
		return nil, err
	}
	return ValidateCustom(defs, vals, create, func(id string) bool {
		var ok bool
		return r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id=$1 AND org_id=$2)`, id, org).Scan(&ok) == nil && ok
	})
}

// SaveFieldDef creates (id "") or updates a definition. Type and key of an
// existing field are immutable so stored values stay valid.
func (r *Repository) SaveFieldDef(ctx context.Context, org, id string, d FieldDef) (FieldDef, error) {
	if err := d.Validate(); err != nil {
		return d, err
	}
	opts, _ := json.Marshal(d.Options)
	if id == "" {
		if d.Entity == "lead" || d.Entity == "contact" {
			var clash bool
			if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.crm_field_defs WHERE org_id=$1 AND key=$2 AND entity IN ('lead','contact'))`, org, d.Key).Scan(&clash); err != nil {
				return d, err
			}
			if clash {
				return d, invalid("a lead/contact field with key %q already exists", d.Key)
			}
		}
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_field_defs (org_id,entity,key,label,type,options,required,position)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, org, d.Entity, d.Key, d.Label, d.Type, opts, d.Required, d.Position).Scan(&d.ID)
		if err != nil && strings.Contains(err.Error(), "23505") {
			return d, invalid("a field with key %q already exists", d.Key)
		}
		return d, err
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.crm_field_defs SET label=$3, options=$4, required=$5, position=$6 WHERE org_id=$1 AND id=$2`,
		org, id, d.Label, opts, d.Required, d.Position)
	if err != nil {
		return d, err
	}
	if ct.RowsAffected() == 0 {
		return d, ErrNotFound
	}
	d.ID = id
	return d, nil
}

// DeleteFieldDef removes a definition and strips its values from records.
func (r *Repository) DeleteFieldDef(ctx context.Context, org, id string) error {
	var entity, key string
	if err := r.pool.QueryRow(ctx, `DELETE FROM adrient.crm_field_defs WHERE org_id=$1 AND id=$2 RETURNING entity, key`, org, id).Scan(&entity, &key); err != nil {
		return ErrNotFound
	}
	tbl := map[string]string{"lead": "crm_people", "contact": "crm_people", "company": "crm_companies", "deal": "crm_deals"}[entity]
	_, err := r.pool.Exec(ctx, `UPDATE adrient.`+tbl+` SET custom = custom - $2 WHERE org_id=$1 AND custom ? $2`, org, key)
	return err
}

// ---- saved views (server side, per user, shareable) ----

// View is a saved filter/sort/column set for a list.
type View struct {
	ID      string          `json:"id"`
	Entity  string          `json:"entity"`
	Name    string          `json:"name"`
	Config  json.RawMessage `json:"config"`
	Shared  bool            `json:"shared"`
	OwnerID string          `json:"owner_id"`
	Mine    bool            `json:"mine"`
}

var viewEntities = map[string]bool{"lead": true, "company": true, "deal": true, "activity": true}

// ListViews returns the caller's views plus views shared by others.
func (r *Repository) ListViews(ctx context.Context, org, user string) ([]View, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, entity, name, config, shared, owner_id::text FROM adrient.crm_views
		WHERE org_id=$1 AND (owner_id=$2 OR shared) ORDER BY lower(name)`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []View{}
	for rows.Next() {
		var v View
		if err := rows.Scan(&v.ID, &v.Entity, &v.Name, &v.Config, &v.Shared, &v.OwnerID); err != nil {
			return nil, err
		}
		v.Mine = v.OwnerID == user
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveView creates or updates one of the caller's own views.
func (r *Repository) SaveView(ctx context.Context, org, user, id string, v View) (View, error) {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || !viewEntities[v.Entity] {
		return v, invalid("a name and a valid entity are required")
	}
	if len(v.Config) == 0 || !json.Valid(v.Config) {
		v.Config = json.RawMessage(`{}`)
	}
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_views (org_id,owner_id,entity,name,config,shared) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
			org, user, v.Entity, v.Name, []byte(v.Config), v.Shared).Scan(&v.ID)
		v.OwnerID, v.Mine = user, true
		return v, err
	}
	ct, err := r.pool.Exec(ctx, `UPDATE adrient.crm_views SET name=$4, config=$5, shared=$6, updated_at=now() WHERE org_id=$1 AND id=$2 AND owner_id=$3`,
		org, id, user, v.Name, []byte(v.Config), v.Shared)
	if err != nil {
		return v, err
	}
	if ct.RowsAffected() == 0 {
		return v, ErrNotFound
	}
	v.ID, v.OwnerID, v.Mine = id, user, true
	return v, nil
}

// DeleteView deletes one of the caller's own views.
func (r *Repository) DeleteView(ctx context.Context, org, user, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_views WHERE org_id=$1 AND id=$2 AND owner_id=$3`, org, id, user)
}

// ---- command search ----

// SearchHit is one command-palette result.
type SearchHit struct {
	Kind  string `json:"kind"` // lead | company | deal | activity
	ID    string `json:"id"`
	Title string `json:"title"`
	Sub   string `json:"sub"`
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Search finds leads/contacts, companies, deals and activities matching q,
// restricted to what the viewer may see.
func (r *Repository) Search(ctx context.Context, org, q string, limit int) ([]SearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []SearchHit{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 8
	}
	pat := "%" + escapeLike(strings.ToLower(q)) + "%"
	v := ViewerFrom(ctx)
	out := []SearchHit{}
	run := func(sql string) error {
		rows, err := r.pool.Query(ctx, sql, org, pat, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h SearchHit
			if err := rows.Scan(&h.Kind, &h.ID, &h.Title, &h.Sub); err != nil {
				return err
			}
			out = append(out, h)
		}
		return rows.Err()
	}
	if err := run(`SELECT 'lead', p.id::text, c.display_name, coalesce(c.emails->>0,'') FROM adrient.crm_people p
		JOIN adrient.contacts c ON c.id=p.contact_id WHERE p.org_id=$1 AND c.trashed_at IS NULL AND ` + v.ownerSQL("p") + `
		AND (lower(c.display_name) LIKE $2 OR lower(c.emails::text) LIKE $2 OR lower(c.phones::text) LIKE $2 OR lower(p.custom::text) LIKE $2)
		ORDER BY c.display_name LIMIT $3`); err != nil {
		return nil, err
	}
	if err := run(`SELECT 'company', co.id::text, co.name, co.industry FROM adrient.crm_companies co WHERE co.org_id=$1 AND ` + v.ownerSQL("co") + `
		AND (lower(co.name) LIKE $2 OR lower(co.website) LIKE $2 OR lower(co.custom::text) LIKE $2) ORDER BY co.name LIMIT $3`); err != nil {
		return nil, err
	}
	if err := run(`SELECT 'deal', d.id::text, d.title, d.stage FROM adrient.crm_deals d WHERE d.org_id=$1 AND ` + v.ownerSQL("d") + `
		AND (lower(d.title) LIKE $2 OR lower(d.custom::text) LIKE $2) ORDER BY d.updated_at DESC LIMIT $3`); err != nil {
		return nil, err
	}
	if err := run(`SELECT 'activity', a.id::text, left(a.summary,80), a.kind FROM adrient.crm_activities a WHERE a.org_id=$1 AND ` + v.childSQL("a") + `
		AND lower(a.summary) LIKE $2 ORDER BY a.occurred_at DESC LIMIT $3`); err != nil {
		return nil, err
	}
	return out, nil
}
