package tickets

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// FieldDef defines one custom field of a project's tickets.
type FieldDef struct {
	ID         string   `json:"id"`
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Kind       string   `json:"kind"` // text | longtext | number | select | date | checkbox
	Options    []string `json:"options"`
	RequiredOn []string `json:"required_on"` // statuses that need a value
	TicketType string   `json:"ticket_type"` // "" = every type
	Position   int      `json:"position"`
}

var fieldKinds = map[string]bool{"text": true, "longtext": true, "number": true, "select": true, "date": true, "checkbox": true}

// Validate normalises and checks a definition.
func (d *FieldDef) Validate() error {
	d.Key = strings.ToLower(strings.TrimSpace(d.Key))
	d.Label = strings.TrimSpace(d.Label)
	if !formKeyOK(d.Key) {
		return errors.New("the key needs 1 to 32 letters, digits or underscores")
	}
	if d.Label == "" || len(d.Label) > 80 {
		return errors.New("a label of up to 80 characters is required")
	}
	if !fieldKinds[d.Kind] {
		return errors.New("unknown field kind")
	}
	if d.Kind == "select" {
		if len(d.Options) < 2 || len(d.Options) > 50 {
			return errors.New("a select field needs between 2 and 50 options")
		}
		seen := map[string]bool{}
		for i, o := range d.Options {
			o = strings.TrimSpace(o)
			if o == "" || len(o) > 80 || seen[o] {
				return errors.New("options must be unique, non-empty and short")
			}
			seen[o] = true
			d.Options[i] = o
		}
	} else {
		d.Options = []string{}
	}
	if d.TicketType != "" && NormalizeType(d.TicketType) != d.TicketType {
		return errors.New("unknown ticket type")
	}
	if d.RequiredOn == nil {
		d.RequiredOn = []string{}
	}
	return nil
}

func (d FieldDef) appliesTo(ticketType string) bool {
	return d.TicketType == "" || d.TicketType == ticketType
}

// ValidateCustom checks values against the definitions that apply to a ticket type. Keys without a
// definition (for example from request forms) pass unchanged. Empty values are dropped.
func ValidateCustom(defs []FieldDef, ticketType string, values map[string]string) (map[string]string, error) {
	out := map[string]string{}
	byKey := map[string]FieldDef{}
	for _, d := range defs {
		byKey[d.Key] = d
	}
	for k, v := range values {
		v = strings.TrimSpace(v)
		if len(k) > 32 || len(v) > 5000 {
			return nil, fmt.Errorf("the value of %q is too long", k)
		}
		d, has := byKey[k]
		if v == "" {
			continue
		}
		if has {
			switch d.Kind {
			case "number":
				if _, err := strconv.ParseFloat(v, 64); err != nil {
					return nil, fmt.Errorf("%s must be a number", d.Label)
				}
			case "date":
				if _, err := time.Parse("2006-01-02", v); err != nil {
					return nil, fmt.Errorf("%s must be a date (YYYY-MM-DD)", d.Label)
				}
			case "checkbox":
				if v != "true" && v != "false" {
					return nil, fmt.Errorf("%s must be yes or no", d.Label)
				}
			case "select":
				ok := false
				for _, o := range d.Options {
					ok = ok || o == v
				}
				if !ok {
					return nil, fmt.Errorf("%s: choose one of the listed options", d.Label)
				}
			}
		}
		out[k] = v
	}
	return out, nil
}

// MissingCustom lists the labels of fields that are required in a status but empty.
func MissingCustom(defs []FieldDef, ticketType, status string, values map[string]string) []string {
	var out []string
	for _, d := range defs {
		if !d.appliesTo(ticketType) {
			continue
		}
		for _, s := range d.RequiredOn {
			if s == status && strings.TrimSpace(values[d.Key]) == "" {
				out = append(out, d.Label)
			}
		}
	}
	return out
}

// ListFieldDefs returns a project's custom field definitions.
func (r *Repository) ListFieldDefs(ctx context.Context, org, projectID string) ([]FieldDef, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, key, label, kind, options, required_on, ticket_type, position FROM adrient.ticket_field_defs
		WHERE org_id=$1 AND project_id=$2 ORDER BY position, label`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FieldDef{}
	for rows.Next() {
		var d FieldDef
		if err := rows.Scan(&d.ID, &d.Key, &d.Label, &d.Kind, &d.Options, &d.RequiredOn, &d.TicketType, &d.Position); err != nil {
			return nil, err
		}
		if d.Options == nil {
			d.Options = []string{}
		}
		if d.RequiredOn == nil {
			d.RequiredOn = []string{}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveFieldDef creates or updates a definition. The key of an existing field cannot change.
func (r *Repository) SaveFieldDef(ctx context.Context, org, projectID string, d FieldDef) (FieldDef, error) {
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return d, err
	}
	if err := d.Validate(); err != nil {
		return d, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var err error
	if d.ID == "" {
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_field_defs WHERE project_id=$1`, projectID).Scan(&n)
		if n >= 40 {
			return d, fmt.Errorf("%w: at most 40 custom fields per project", ErrInvalid)
		}
		d.Position = n
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_field_defs (org_id, project_id, key, label, kind, options, required_on, ticket_type, position)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, org, projectID, d.Key, d.Label, d.Kind, d.Options, d.RequiredOn, d.TicketType, d.Position).Scan(&d.ID)
		if err != nil && strings.Contains(err.Error(), "duplicate key") {
			return d, fmt.Errorf("%w: a field with this key already exists", ErrInvalid)
		}
	} else {
		var id string
		err = r.pool.QueryRow(ctx, `UPDATE adrient.ticket_field_defs SET label=$4, kind=$5, options=$6, required_on=$7, ticket_type=$8, position=$9
			WHERE id=$1 AND org_id=$2 AND project_id=$3 RETURNING id::text`, d.ID, org, projectID, d.Label, d.Kind, d.Options, d.RequiredOn, d.TicketType, d.Position).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return d, ErrNotFound
		}
	}
	return d, err
}

// DeleteFieldDef removes a definition; values already on tickets stay but are no longer shown as a field.
func (r *Repository) DeleteFieldDef(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_field_defs WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// OOO is a person's out-of-office end as stored for the desk.
func (r *Repository) GetOOO(ctx context.Context, org, user string) (*time.Time, error) {
	var until *time.Time
	err := r.pool.QueryRow(ctx, `SELECT max(ooo_until) FROM adrient.ticket_group_members WHERE org_id=$1 AND user_id=$2`, org, user).Scan(&until)
	if until != nil && !until.After(time.Now()) {
		return nil, err
	}
	return until, err
}
