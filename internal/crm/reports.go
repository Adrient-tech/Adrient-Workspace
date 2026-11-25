package crm

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Report is a table: the UI renders it and the CSV export is the same data.
type Report struct {
	Name    string   `json:"name"`
	Title   string   `json:"title"`
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// CSVSafe prefixes cells that a spreadsheet would run as a formula.
func CSVSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func cell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return CSVSafe(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case time.Time:
		return t.Format("2006-01-02")
	case []string:
		return CSVSafe(strings.Join(t, "; "))
	}
	return CSVSafe(fmt.Sprint(v))
}

// CSV renders the report.
func (rp Report) CSV() []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write(rp.Columns)
	for _, row := range rp.Rows {
		rec := make([]string, len(row))
		for i, v := range row {
			rec[i] = cell(v)
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return b.Bytes()
}

// ReportNames lists the available reports.
var ReportNames = []string{"velocity", "winrate_source", "winrate_owner", "aging", "activity", "forecast"}

// RunReport builds a named report from the deals the viewer may see.
func (r *Repository) RunReport(ctx context.Context, org, name string, params map[string]string) (Report, error) {
	v := ViewerFrom(ctx)
	vis := v.ownerSQL("d")
	rp := Report{Name: name}
	query := func(q string, args ...any) error {
		rows, err := r.pool.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				return err
			}
			for i, x := range vals {
				if x == nil {
					continue
				}
				switch t := x.(type) {
				case [16]byte:
					vals[i] = fmt.Sprintf("%x-%x-%x-%x-%x", t[0:4], t[4:6], t[6:8], t[8:10], t[10:])
				case time.Time:
					vals[i] = t.Format("2006-01-02")
				}
			}
			rp.Rows = append(rp.Rows, vals)
		}
		return rows.Err()
	}
	switch name {
	case "velocity":
		rp.Title = "Pipeline velocity: days spent in each stage"
		rp.Columns = []string{"Stage", "Deals that passed", "Average days", "Median days"}
		err := query(`WITH s AS (
			SELECT h.deal_id, h.stage, extract(epoch FROM coalesce(lead(h.entered_at) OVER (PARTITION BY h.deal_id ORDER BY h.entered_at), now()) - h.entered_at)/86400.0 AS days
			FROM adrient.crm_stage_history h JOIN adrient.crm_deals d ON d.id=h.deal_id WHERE h.org_id=$1 AND `+vis+`)
			SELECT stage, count(*)::int, round(avg(days)::numeric,1)::float8, round((percentile_cont(0.5) WITHIN GROUP (ORDER BY days))::numeric,1)::float8
			FROM s GROUP BY stage ORDER BY 3 DESC`, org)
		if err != nil {
			return rp, err
		}
		var cyc float64
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*), coalesce(round(avg(extract(epoch FROM d.closed_at - d.created_at)/86400.0)::numeric,1),0)::float8
			FROM adrient.crm_deals d WHERE d.org_id=$1 AND d.stage='won' AND d.closed_at IS NOT NULL AND `+vis, org).Scan(&n, &cyc)
		rp.Rows = append(rp.Rows, []any{"Whole sales cycle (won deals)", n, cyc, nil})
	case "winrate_source", "winrate_owner":
		col := "coalesce(nullif(d.source,''),'(none)')"
		rp.Title = "Win rate by source"
		rp.Columns = []string{"Source", "Won", "Lost", "Win rate %", "Won value"}
		join := ""
		if name == "winrate_owner" {
			col = "coalesce(u.display_name,'(unassigned)')"
			join = "LEFT JOIN adrient.users u ON u.id=d.owner_id"
			rp.Title = "Win rate by owner"
			rp.Columns[0] = "Owner"
		}
		if err := query(`SELECT `+col+` AS k, count(*) FILTER (WHERE d.stage='won')::int, count(*) FILTER (WHERE d.stage='lost')::int,
			CASE WHEN count(*) FILTER (WHERE d.stage IN ('won','lost'))=0 THEN 0 ELSE
			round(100.0*count(*) FILTER (WHERE d.stage='won')/count(*) FILTER (WHERE d.stage IN ('won','lost')),1) END::float8,
			coalesce(sum(d.value) FILTER (WHERE d.stage='won'),0)::float8
			FROM adrient.crm_deals d `+join+` WHERE d.org_id=$1 AND d.stage IN ('won','lost') AND `+vis+` GROUP BY k ORDER BY 2 DESC, k`, org); err != nil {
			return rp, err
		}
	case "aging":
		rp.Title = "Aging open deals"
		rp.Columns = []string{"Deal", "Stage", "Owner", "Age (days)", "Days in stage", "Value", "Bucket"}
		if err := query(`SELECT d.title, d.stage, coalesce(u.display_name,''), floor(extract(epoch FROM now()-d.created_at)/86400)::int,
			floor(extract(epoch FROM now()-d.stage_entered_at)/86400)::int, d.value::float8,
			CASE WHEN now()-d.created_at < interval '31 days' THEN '0-30' WHEN now()-d.created_at < interval '61 days' THEN '31-60'
			     WHEN now()-d.created_at < interval '91 days' THEN '61-90' ELSE '90+' END
			FROM adrient.crm_deals d LEFT JOIN adrient.users u ON u.id=d.owner_id
			WHERE d.org_id=$1 AND d.stage NOT IN ('won','lost') AND `+vis+` ORDER BY d.created_at LIMIT 500`, org); err != nil {
			return rp, err
		}
	case "activity":
		days := 30
		if n, err := strconv.Atoi(params["days"]); err == nil && n > 0 && n <= 365 {
			days = n
		}
		rp.Title = fmt.Sprintf("Activity by person, last %d days", days)
		rp.Columns = []string{"User", "Calls", "Meetings", "Emails", "Notes", "Total"}
		if err := query(`SELECT coalesce(u.display_name,'(unknown)'), count(*) FILTER (WHERE a.kind='call')::int, count(*) FILTER (WHERE a.kind='meeting')::int,
			count(*) FILTER (WHERE a.kind='email')::int, count(*) FILTER (WHERE a.kind='note')::int, count(*)::int
			FROM adrient.crm_activities a LEFT JOIN adrient.users u ON u.id=a.user_id
			WHERE a.org_id=$1 AND a.occurred_at > now() - make_interval(days => $2) AND `+v.childSQL("a")+` GROUP BY 1 ORDER BY 6 DESC, 1`, org, days); err != nil {
			return rp, err
		}
	case "forecast":
		rp.Title = "Revenue forecast by team and month (stage-weighted)"
		rp.Columns = []string{"Team", "Month", "Open value", "Weighted forecast", "Won value"}
		set, err := r.GetSettings(ctx, org)
		if err != nil {
			return rp, err
		}
		weights := StageWeights(set.Stages)
		rows, err := r.pool.Query(ctx, `SELECT coalesce((SELECT t.name FROM adrient.crm_team_members m JOIN adrient.crm_teams t ON t.id=m.team_id
			WHERE m.user_id=d.owner_id ORDER BY t.name LIMIT 1),'(no team)'), to_char(coalesce(d.expected_close, d.created_at::date),'YYYY-MM'), d.stage, d.value::float8
			FROM adrient.crm_deals d WHERE d.org_id=$1 AND d.stage<>'lost' AND `+vis, org)
		if err != nil {
			return rp, err
		}
		type k struct{ team, month string }
		agg := map[k][3]float64{}
		for rows.Next() {
			var team, month, stage string
			var val float64
			if err := rows.Scan(&team, &month, &stage, &val); err != nil {
				rows.Close()
				return rp, err
			}
			a := agg[k{team, month}]
			if stage == StageWon {
				a[2] += val
			} else {
				a[0] += val
				a[1] += val * weights[stage]
			}
			agg[k{team, month}] = a
		}
		rows.Close()
		keys := make([]k, 0, len(agg))
		for kk := range agg {
			keys = append(keys, kk)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].team != keys[j].team {
				return keys[i].team < keys[j].team
			}
			return keys[i].month < keys[j].month
		})
		for _, kk := range keys {
			a := agg[kk]
			rp.Rows = append(rp.Rows, []any{kk.team, kk.month, round2(a[0]), round2(a[1]), round2(a[2])})
		}
	default:
		return rp, invalid("unknown report %q", name)
	}
	if rp.Rows == nil {
		rp.Rows = [][]any{}
	}
	return rp, nil
}

// StageWeights gives each open stage a win probability rising linearly with its position.
func StageWeights(stages []Stage) map[string]float64 {
	w := map[string]float64{}
	for i, s := range stages {
		w[s.ID] = float64(i+1) / float64(len(stages)+1)
	}
	return w
}

// ---- export ----

// ExportEntities are the exportable record kinds.
var ExportEntities = []string{"leads", "companies", "deals", "activities"}

// Export returns one entity as a report (columns include every custom field).
func (r *Repository) Export(ctx context.Context, org, entity string) (Report, error) {
	rp := Report{Name: entity, Title: entity}
	defs, err := r.ListFieldDefs(ctx, org)
	if err != nil {
		return rp, err
	}
	var kind string
	switch entity {
	case "leads":
		kind = "person"
	case "companies":
		kind = "company"
	case "deals":
		kind = "deal"
	}
	var base []string
	var raw json.RawMessage
	switch entity {
	case "leads":
		base = []string{"id", "display_name", "emails", "phones", "job_title", "status", "source", "tags", "notes", "company_id", "owner_id", "score", "created_at"}
		raw, err = r.ListPeople(ctx, org)
	case "companies":
		base = []string{"id", "name", "industry", "website", "phone", "address", "tags", "notes", "owner_id", "created_at"}
		raw, err = r.ListCompanies(ctx, org)
	case "deals":
		base = []string{"id", "title", "company_id", "contact_id", "stage", "value", "currency", "billing", "expected_close", "source", "owner_id", "close_reason", "created_at"}
		raw, err = r.ListDeals(ctx, org)
	case "activities":
		base = []string{"id", "kind", "summary", "occurred_at", "deal_id", "contact_id", "company_id", "direction", "duration_sec", "user_id"}
		raw, err = r.ListActivities(ctx, org)
	default:
		return rp, invalid("unknown export %q", entity)
	}
	if err != nil {
		return rp, err
	}
	var custom []FieldDef
	for _, d := range defs {
		if kind != "" && (d.Entity == kind || (kind == "person" && (d.Entity == "lead" || d.Entity == "contact"))) {
			custom = append(custom, d)
		}
	}
	rp.Columns = append([]string{}, base...)
	for _, d := range custom {
		rp.Columns = append(rp.Columns, "custom:"+d.Key)
	}
	var recs []map[string]any
	if err := json.Unmarshal(raw, &recs); err != nil {
		return rp, err
	}
	for _, m := range recs {
		row := make([]any, 0, len(rp.Columns))
		for _, c := range base {
			row = append(row, flatten(m[c]))
		}
		cv, _ := m["custom"].(map[string]any)
		for _, d := range custom {
			row = append(row, flatten(cv[d.Key]))
		}
		rp.Rows = append(rp.Rows, row)
	}
	if rp.Rows == nil {
		rp.Rows = [][]any{}
	}
	return rp, nil
}

func flatten(v any) any {
	switch t := v.(type) {
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, fmt.Sprint(e))
		}
		return strings.Join(parts, "; ")
	case map[string]any:
		b, _ := json.Marshal(t)
		return string(b)
	}
	return v
}
