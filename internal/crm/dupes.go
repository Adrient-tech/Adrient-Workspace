package crm

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var nonDigit = regexp.MustCompile(`\D`)
var nonAlnum = regexp.MustCompile(`[^a-z0-9 ]`)

// NormEmail lower-cases and trims an address.
func NormEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// NormPhone keeps the last 10 digits, so +91 98xxx and 098xxx compare equal.
func NormPhone(s string) string {
	d := nonDigit.ReplaceAllString(s, "")
	if len(d) < 7 {
		return ""
	}
	if len(d) > 10 {
		d = d[len(d)-10:]
	}
	return d
}

var companySuffix = map[string]bool{"inc": true, "llc": true, "ltd": true, "limited": true, "pvt": true, "private": true, "llp": true, "corp": true, "corporation": true, "co": true, "company": true, "gmbh": true, "the": true}

// NormCompany strips punctuation and legal suffixes: "Acme Pvt. Ltd." == "acme".
func NormCompany(s string) string {
	s = nonAlnum.ReplaceAllString(strings.ToLower(s), " ")
	var out []string
	for _, w := range strings.Fields(s) {
		if !companySuffix[w] {
			out = append(out, w)
		}
	}
	return strings.Join(out, " ")
}

// WebsiteDomain returns the bare host of a URL or domain ("https://www.acme.com/x" -> "acme.com").
func WebsiteDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	s = strings.TrimPrefix(s, "www.")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return s
}

// DupGroup is a set of records that look like the same entity.
type DupGroup struct {
	Kind   string   `json:"kind"` // person | company
	Reason string   `json:"reason"`
	IDs    []string `json:"ids"`
	Names  []string `json:"names"`
}

type dupItem struct {
	id, name string
	keys     []string // "email:x", "phone:y", "name:z", "domain:d"
}

// GroupDuplicates links items that share any key (union-find) and returns the groups with 2+ members.
func GroupDuplicates(kind string, items []dupItem) []DupGroup {
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" || parent[x] == x {
			parent[x] = x
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	keyOwner := map[string]string{}
	reason := map[string]string{}
	for _, it := range items {
		find(it.id)
		for _, k := range it.keys {
			if o, ok := keyOwner[k]; ok {
				parent[find(it.id)] = find(o)
				reason[k] = strings.SplitN(k, ":", 2)[0]
			} else {
				keyOwner[k] = it.id
			}
		}
	}
	groups := map[string]*DupGroup{}
	names := map[string]string{}
	for _, it := range items {
		names[it.id] = it.name
		root := find(it.id)
		g := groups[root]
		if g == nil {
			g = &DupGroup{Kind: kind}
			groups[root] = g
		}
		g.IDs = append(g.IDs, it.id)
		g.Names = append(g.Names, it.name)
	}
	rs := map[string]map[string]bool{}
	for k, why := range reason {
		root := find(keyOwner[k])
		if rs[root] == nil {
			rs[root] = map[string]bool{}
		}
		rs[root][why] = true
	}
	out := []DupGroup{}
	for root, g := range groups {
		if len(g.IDs) < 2 {
			continue
		}
		var rr []string
		for w := range rs[root] {
			rr = append(rr, w)
		}
		sort.Strings(rr)
		g.Reason = strings.Join(rr, "+")
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Names[0] < out[j].Names[0] })
	return out
}

// FindDuplicates returns duplicate groups of people (same email/phone) and companies (same name/domain).
func (r *Repository) FindDuplicates(ctx context.Context, org string) ([]DupGroup, error) {
	v := ViewerFrom(ctx)
	rows, err := r.pool.Query(ctx, `SELECT p.id::text, c.display_name, c.emails, c.phones FROM adrient.crm_people p
		JOIN adrient.contacts c ON c.id=p.contact_id WHERE p.org_id=$1 AND c.trashed_at IS NULL AND `+v.ownerSQL("p"), org)
	if err != nil {
		return nil, err
	}
	var people []dupItem
	for rows.Next() {
		var it dupItem
		var e, p []byte
		if err := rows.Scan(&it.id, &it.name, &e, &p); err != nil {
			rows.Close()
			return nil, err
		}
		var es, ps []string
		_ = json.Unmarshal(e, &es)
		_ = json.Unmarshal(p, &ps)
		for _, x := range es {
			if n := NormEmail(x); n != "" {
				it.keys = append(it.keys, "email:"+n)
			}
		}
		for _, x := range ps {
			if n := NormPhone(x); n != "" {
				it.keys = append(it.keys, "phone:"+n)
			}
		}
		people = append(people, it)
	}
	rows.Close()
	crows, err := r.pool.Query(ctx, `SELECT co.id::text, co.name, co.website FROM adrient.crm_companies co WHERE co.org_id=$1 AND `+v.ownerSQL("co"), org)
	if err != nil {
		return nil, err
	}
	var comps []dupItem
	for crows.Next() {
		var it dupItem
		var web string
		if err := crows.Scan(&it.id, &it.name, &web); err != nil {
			crows.Close()
			return nil, err
		}
		if n := NormCompany(it.name); n != "" {
			it.keys = append(it.keys, "name:"+n)
		}
		if d := WebsiteDomain(web); d != "" {
			it.keys = append(it.keys, "domain:"+d)
		}
		comps = append(comps, it)
	}
	crows.Close()
	return append(GroupDuplicates("person", people), GroupDuplicates("company", comps)...), nil
}

// MergeCompanies folds loser into winner: records move, tags and custom values merge
// (winner wins on conflicts), then the loser is deleted. A merge-log row keeps a snapshot.
func (r *Repository) MergeCompanies(ctx context.Context, org, winner, loser string) error {
	if winner == loser {
		return invalid("choose two different companies")
	}
	v := ViewerFrom(ctx)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.crm_companies co WHERE org_id=$1 AND id IN ($2,$3) AND `+v.ownerSQL("co"), org, winner, loser).Scan(&n); err != nil || n != 2 {
		return ErrNotFound
	}
	var snap []byte
	_ = tx.QueryRow(ctx, `SELECT row_to_json(c) FROM adrient.crm_companies c WHERE id=$1`, loser).Scan(&snap)
	stmts := []string{
		`UPDATE adrient.crm_people SET company_id=$2 WHERE org_id=$1 AND company_id=$3`,
		`UPDATE adrient.crm_deals SET company_id=$2 WHERE org_id=$1 AND company_id=$3`,
		`UPDATE adrient.crm_activities SET company_id=$2 WHERE org_id=$1 AND company_id=$3`,
		`UPDATE adrient.crm_companies w SET tags=(SELECT coalesce(array_agg(DISTINCT t),'{}') FROM unnest(w.tags || l.tags) t),
		   custom=l.custom || w.custom, industry=CASE WHEN w.industry='' THEN l.industry ELSE w.industry END,
		   website=CASE WHEN w.website='' THEN l.website ELSE w.website END, phone=CASE WHEN w.phone='' THEN l.phone ELSE w.phone END,
		   address=CASE WHEN w.address='' THEN l.address ELSE w.address END,
		   notes=trim(both E'\n' from w.notes || CASE WHEN l.notes<>'' THEN E'\n' || l.notes ELSE '' END)
		   FROM adrient.crm_companies l WHERE w.org_id=$1 AND w.id=$2 AND l.org_id=$1 AND l.id=$3`,
		`UPDATE adrient.crm_external_ids SET record_id=$2 WHERE org_id=$1 AND entity='company' AND record_id=$3`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s, org, winner, loser); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.crm_companies WHERE org_id=$1 AND id=$2`, org, loser); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.crm_merge_log (org_id,kind,winner_id,loser_id,snapshot,user_id) VALUES ($1,'company',$2,$3,$4,$5)`,
		org, winner, loser, snap, uidOrNil(ctx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MergePeople folds loser into winner (both crm_people ids). Deals, follow-ups,
// activities and logged mail move to the winner; the loser's emails, phones and
// tags are added to the winner; the loser's Contact is moved to trash (reversible).
func (r *Repository) MergePeople(ctx context.Context, org, winner, loser string) error {
	if winner == loser {
		return invalid("choose two different people")
	}
	v := ViewerFrom(ctx)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var wc, lc string
	rows, err := tx.Query(ctx, `SELECT p.id::text, p.contact_id::text FROM adrient.crm_people p WHERE p.org_id=$1 AND p.id IN ($2,$3) AND `+v.ownerSQL("p"), org, winner, loser)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, cid string
		_ = rows.Scan(&id, &cid)
		if id == winner {
			wc = cid
		} else {
			lc = cid
		}
	}
	rows.Close()
	if wc == "" || lc == "" {
		return ErrNotFound
	}
	var snap []byte
	_ = tx.QueryRow(ctx, `SELECT json_build_object('person', row_to_json(p), 'contact', row_to_json(c)) FROM adrient.crm_people p
		JOIN adrient.contacts c ON c.id=p.contact_id WHERE p.id=$1`, loser).Scan(&snap)
	stmts := []string{
		`UPDATE adrient.crm_deals SET contact_id=$2 WHERE org_id=$1 AND contact_id=$3`,
		`UPDATE adrient.crm_followups SET contact_id=$2 WHERE org_id=$1 AND contact_id=$3`,
		`UPDATE adrient.crm_activities SET contact_id=$2 WHERE org_id=$1 AND contact_id=$3`,
		`DELETE FROM adrient.crm_deal_people l USING adrient.crm_deal_people w WHERE $1::text<>'' AND l.contact_id=$3 AND w.contact_id=$2 AND l.deal_id=w.deal_id`,
		`UPDATE adrient.crm_deal_people SET contact_id=$2 WHERE $1::text<>'' AND contact_id=$3`,
		`UPDATE adrient.contacts w SET emails=(SELECT coalesce(jsonb_agg(DISTINCT e),'[]') FROM jsonb_array_elements_text(w.emails || l.emails) e),
		   phones=(SELECT coalesce(jsonb_agg(DISTINCT e),'[]') FROM jsonb_array_elements_text(w.phones || l.phones) e),
		   company=CASE WHEN w.company='' THEN l.company ELSE w.company END, job_title=CASE WHEN w.job_title='' THEN l.job_title ELSE w.job_title END,
		   updated_at=now() FROM adrient.contacts l WHERE w.org_id=$1 AND w.id=$2 AND l.org_id=$1 AND l.id=$3`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(ctx, s, org, wc, lc); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.crm_mail_log SET person_id=$2 WHERE org_id=$1 AND person_id=$3 AND NOT EXISTS
		(SELECT 1 FROM adrient.crm_mail_log x WHERE x.message_id=adrient.crm_mail_log.message_id AND x.person_id=$2)`, org, winner, loser); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.crm_people w SET tags=(SELECT coalesce(array_agg(DISTINCT t),'{}') FROM unnest(w.tags || l.tags) t),
		custom=l.custom || w.custom, source=CASE WHEN w.source='' THEN l.source ELSE w.source END,
		company_id=coalesce(w.company_id, l.company_id), owner_id=coalesce(w.owner_id, l.owner_id),
		notes=trim(both E'\n' from w.notes || CASE WHEN l.notes<>'' THEN E'\n' || l.notes ELSE '' END)
		FROM adrient.crm_people l WHERE w.org_id=$1 AND w.id=$2 AND l.org_id=$1 AND l.id=$3`, org, winner, loser); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.crm_external_ids SET record_id=$2 WHERE org_id=$1 AND entity='lead' AND record_id=$3`, org, winner, loser); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.crm_people WHERE org_id=$1 AND id=$2`, org, loser); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.contacts SET trashed_at=now() WHERE org_id=$1 AND id=$2`, org, lc); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.crm_merge_log (org_id,kind,winner_id,loser_id,snapshot,user_id) VALUES ($1,'person',$2,$3,$4,$5)`,
		org, winner, loser, snap, uidOrNil(ctx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_ = r.RescorePerson(ctx, org, winner)
	return nil
}
