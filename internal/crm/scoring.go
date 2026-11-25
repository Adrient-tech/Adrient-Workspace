package crm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ScoreRule adds points to a lead when a condition holds.
type ScoreRule struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Field  string `json:"field"` // status | source | tag | has_email | has_phone | has_company | activities_30d | custom:<key>
	Op     string `json:"op"`    // eq | contains | gt | lt | exists
	Value  string `json:"value"`
	Points int    `json:"points"`
}

var scoreFields = map[string]bool{"status": true, "source": true, "tag": true, "has_email": true, "has_phone": true, "has_company": true, "activities_30d": true}
var scoreOps = map[string]bool{"eq": true, "contains": true, "gt": true, "lt": true, "exists": true}

// Validate checks a score rule.
func (s *ScoreRule) Validate() error {
	if !scoreFields[s.Field] && !strings.HasPrefix(s.Field, "custom:") {
		return invalid("unknown score field %q", s.Field)
	}
	if !scoreOps[s.Op] {
		return invalid("unknown operator %q", s.Op)
	}
	if s.Points < -100 || s.Points > 100 || s.Points == 0 {
		return invalid("points must be between -100 and 100, not 0")
	}
	if s.Op == "gt" || s.Op == "lt" {
		if _, err := strconv.ParseFloat(s.Value, 64); err != nil {
			return invalid("gt/lt need a number")
		}
	}
	return nil
}

// ScoreAttrs are the facts about a lead that score rules read.
type ScoreAttrs map[string]any

func attrString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	case []string:
		return strings.Join(t, "\x1f")
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, fmt.Sprint(e))
		}
		return strings.Join(parts, "\x1f")
	}
	return ""
}

// EvalScore sums the points of every rule whose condition holds.
func EvalScore(a ScoreAttrs, rules []ScoreRule) int {
	total := 0
	for _, r := range rules {
		v, ok := a[r.Field]
		s := attrString(v)
		hit := false
		switch r.Op {
		case "exists":
			hit = ok && s != "" && s != "false" && s != "0"
		case "eq":
			if parts := strings.Split(s, "\x1f"); len(parts) > 1 || r.Field == "tag" {
				for _, p := range parts {
					if strings.EqualFold(p, r.Value) {
						hit = true
					}
				}
			} else {
				hit = strings.EqualFold(s, r.Value)
			}
		case "contains":
			hit = r.Value != "" && strings.Contains(strings.ToLower(s), strings.ToLower(r.Value))
		case "gt", "lt":
			x, e1 := strconv.ParseFloat(s, 64)
			y, e2 := strconv.ParseFloat(r.Value, 64)
			if e1 == nil && e2 == nil {
				hit = (r.Op == "gt" && x > y) || (r.Op == "lt" && x < y)
			}
		}
		if hit {
			total += r.Points
		}
	}
	return total
}

func (r *Repository) ListScoreRules(ctx context.Context, org string) ([]ScoreRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, field, op, value, points FROM adrient.crm_score_rules WHERE org_id=$1 ORDER BY name`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScoreRule{}
	for rows.Next() {
		var s ScoreRule
		if err := rows.Scan(&s.ID, &s.Name, &s.Field, &s.Op, &s.Value, &s.Points); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) SaveScoreRule(ctx context.Context, org, id string, s ScoreRule) (ScoreRule, error) {
	if err := s.Validate(); err != nil {
		return s, err
	}
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_score_rules (org_id,name,field,op,value,points) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
			org, s.Name, s.Field, s.Op, s.Value, s.Points).Scan(&s.ID)
		return s, err
	}
	if err := r.del(ctx, `UPDATE adrient.crm_score_rules SET name=$3,field=$4,op=$5,value=$6,points=$7 WHERE org_id=$1 AND id=$2`, org, id, s.Name, s.Field, s.Op, s.Value, s.Points); err != nil {
		return s, err
	}
	s.ID = id
	return s, nil
}

func (r *Repository) DeleteScoreRule(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_score_rules WHERE org_id=$1 AND id=$2`, org, id)
}

// personAttrs reads the facts about one person.
func (r *Repository) personAttrs(ctx context.Context, org, personID string) (ScoreAttrs, error) {
	var status, source string
	var tags []string
	var emails, phones, custom []byte
	var company *string
	var acts int
	err := r.pool.QueryRow(ctx, `SELECT p.status, p.source, p.tags, c.emails, c.phones, p.custom, p.company_id::text,
		(SELECT count(*) FROM adrient.crm_activities a WHERE a.org_id=p.org_id AND a.contact_id=p.contact_id AND a.occurred_at > now() - interval '30 days')
		FROM adrient.crm_people p JOIN adrient.contacts c ON c.id=p.contact_id WHERE p.org_id=$1 AND p.id=$2`, org, personID).
		Scan(&status, &source, &tags, &emails, &phones, &custom, &company, &acts)
	if err != nil {
		return nil, err
	}
	var em, ph []string
	_ = json.Unmarshal(emails, &em)
	_ = json.Unmarshal(phones, &ph)
	a := ScoreAttrs{"status": status, "source": source, "tag": tags, "has_email": len(em) > 0, "has_phone": len(ph) > 0,
		"has_company": company != nil, "activities_30d": acts}
	var cv map[string]any
	_ = json.Unmarshal(custom, &cv)
	for k, v := range cv {
		a["custom:"+k] = v
	}
	return a, nil
}

// RescorePerson recomputes and stores one lead's score.
func (r *Repository) RescorePerson(ctx context.Context, org, personID string) error {
	if personID == "" {
		return nil
	}
	rules, err := r.ListScoreRules(ctx, org)
	if err != nil {
		return err
	}
	a, err := r.personAttrs(ctx, org, personID)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `UPDATE adrient.crm_people SET score=$3 WHERE org_id=$1 AND id=$2`, org, personID, EvalScore(a, rules))
	return err
}

// RescoreAll recomputes every lead's score in the org and returns how many were updated.
func (r *Repository) RescoreAll(ctx context.Context, org string) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM adrient.crm_people WHERE org_id=$1`, org)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := r.RescorePerson(ctx, org, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}
