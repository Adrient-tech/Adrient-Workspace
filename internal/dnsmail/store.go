package dnsmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Settings is the per-domain mail configuration (mail_domain_settings).
type Settings struct {
	DMARCPolicy string   `json:"dmarc_policy"`
	DMARCRua    string   `json:"dmarc_rua"`
	MTASTSMode  string   `json:"mta_sts_mode"`
	MTASTSID    string   `json:"mta_sts_id"`
	TLSRPTRua   string   `json:"tls_rpt_rua"`
	MailHost    string   `json:"mail_host"`
	SPFIPs      []string `json:"spf_ips"`
}

var (
	hostRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	ipRe   = regexp.MustCompile(`^[0-9a-fA-F:.]{3,45}(/[0-9]{1,3})?$`)
)

// Validate checks and normalises settings.
func (s *Settings) Validate() error {
	switch s.DMARCPolicy {
	case "none", "quarantine", "reject":
	default:
		return errors.New("dmarc_policy must be none, quarantine or reject")
	}
	switch s.MTASTSMode {
	case "none", "testing", "enforce":
	default:
		return errors.New("mta_sts_mode must be none, testing or enforce")
	}
	for _, a := range []string{s.DMARCRua, s.TLSRPTRua} {
		if a == "" {
			continue
		}
		if _, err := mail.ParseAddress(a); err != nil || strings.ContainsAny(a, ";, \r\n") {
			return errors.New("report address is not a valid e-mail address")
		}
	}
	s.MailHost = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s.MailHost), "."))
	if s.MailHost != "" && !hostRe.MatchString(s.MailHost) {
		return errors.New("mail_host is not a valid host name")
	}
	for i, ip := range s.SPFIPs {
		s.SPFIPs[i] = strings.TrimSpace(ip)
		if !ipRe.MatchString(s.SPFIPs[i]) {
			return errors.New("spf_ips contains an invalid address")
		}
	}
	return nil
}

// Store persists DKIM keys, settings and check history.
type Store struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// DomainRef identifies a verified domain.
type DomainRef struct{ ID, OrgID, Domain string }

// Verified finds a verified domain by name across orgs (public endpoints).
func (s *Store) Verified(ctx context.Context, name string) (DomainRef, bool) {
	var d DomainRef
	err := s.Pool.QueryRow(ctx, `SELECT id::text, org_id::text, domain FROM adrient.org_domains
		WHERE domain = $1 AND status = 'verified'`, strings.ToLower(name)).Scan(&d.ID, &d.OrgID, &d.Domain)
	return d, err == nil
}

// GetSettings returns the settings, or defaults when none are stored.
func (s *Store) GetSettings(ctx context.Context, domainID string) (Settings, error) {
	st := Settings{DMARCPolicy: "none", MTASTSMode: "testing", MTASTSID: "1", SPFIPs: []string{}}
	err := s.Pool.QueryRow(ctx, `SELECT dmarc_policy, dmarc_rua, mta_sts_mode, mta_sts_id, tls_rpt_rua, mail_host, spf_ips
		FROM adrient.mail_domain_settings WHERE domain_id = $1::uuid`, domainID).
		Scan(&st.DMARCPolicy, &st.DMARCRua, &st.MTASTSMode, &st.MTASTSID, &st.TLSRPTRua, &st.MailHost, &st.SPFIPs)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	return st, err
}

// PutSettings upserts settings. When the MTA-STS mode changes the policy id is
// bumped so receivers refetch the policy.
func (s *Store) PutSettings(ctx context.Context, domainID string, in Settings) (Settings, error) {
	if err := in.Validate(); err != nil {
		return Settings{}, err
	}
	old, err := s.GetSettings(ctx, domainID)
	if err != nil {
		return Settings{}, err
	}
	in.MTASTSID = old.MTASTSID
	if in.MTASTSMode != old.MTASTSMode || in.MailHost != old.MailHost {
		in.MTASTSID = s.now().UTC().Format("20060102T150405")
	}
	if in.SPFIPs == nil {
		in.SPFIPs = []string{}
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO adrient.mail_domain_settings
		(domain_id, dmarc_policy, dmarc_rua, mta_sts_mode, mta_sts_id, tls_rpt_rua, mail_host, spf_ips, updated_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,now())
		ON CONFLICT (domain_id) DO UPDATE SET dmarc_policy=$2, dmarc_rua=$3, mta_sts_mode=$4, mta_sts_id=$5,
		tls_rpt_rua=$6, mail_host=$7, spf_ips=$8, updated_at=now()`,
		domainID, in.DMARCPolicy, in.DMARCRua, in.MTASTSMode, in.MTASTSID, in.TLSRPTRua, in.MailHost, in.SPFIPs)
	return in, err
}

const keyCols = `id::text, domain_id::text, selector, algo, public_key, state, created_at, activated_at, rotated_at, private_key_enc`

func scanKey(row pgx.Row) (Key, error) {
	var k Key
	err := row.Scan(&k.ID, &k.DomainID, &k.Selector, &k.Algo, &k.PublicKey, &k.State, &k.CreatedAt, &k.ActivatedAt, &k.RotatedAt, &k.Private)
	return k, err
}

// Keys lists the non-retired-first keys of a domain (private part still sealed).
func (s *Store) Keys(ctx context.Context, domainID string) ([]Key, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+keyCols+` FROM adrient.dkim_keys WHERE domain_id = $1::uuid
		ORDER BY created_at DESC`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Key
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CreateNext generates a key in state next (replacing an unpublished next key).
func (s *Store) CreateNext(ctx context.Context, secret, domainID, algo string) (Key, error) {
	if algo == "" {
		algo = AlgoRSA
	}
	pub, priv, err := GenerateKey(algo)
	if err != nil {
		return Key{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Key{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Serialise per domain so two clicks do not collide on the unique indexes.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "dkim:"+domainID); err != nil {
		return Key{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.dkim_keys WHERE domain_id=$1::uuid AND state='next'`, domainID); err != nil {
		return Key{}, err
	}
	rows, err := tx.Query(ctx, `SELECT selector FROM adrient.dkim_keys WHERE domain_id=$1::uuid`, domainID)
	if err != nil {
		return Key{}, err
	}
	taken := map[string]bool{}
	for rows.Next() {
		var sel string
		_ = rows.Scan(&sel)
		taken[sel] = true
	}
	rows.Close()
	sel := Selector(s.now(), func(x string) bool { return taken[x] })
	enc, err := Encrypt(secret, priv, []byte(domainID+"|"+sel))
	if err != nil {
		return Key{}, err
	}
	k, err := scanKey(tx.QueryRow(ctx, `INSERT INTO adrient.dkim_keys (domain_id, selector, algo, public_key, private_key_enc, state, created_at)
		VALUES ($1::uuid,$2,$3,$4,$5,'next',$6) RETURNING `+keyCols, domainID, sel, algo, pub, enc, s.now()))
	if err != nil {
		return Key{}, err
	}
	return k, tx.Commit(ctx)
}

// Activate flips next to active and retires the previous active key.
func (s *Store) Activate(ctx context.Context, domainID string) (Key, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Key{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "dkim:"+domainID); err != nil {
		return Key{}, err
	}
	now := s.now()
	if _, err := tx.Exec(ctx, `UPDATE adrient.dkim_keys SET state='retired', rotated_at=$2
		WHERE domain_id=$1::uuid AND state='active'`, domainID, now); err != nil {
		return Key{}, err
	}
	k, err := scanKey(tx.QueryRow(ctx, `UPDATE adrient.dkim_keys SET state='active', activated_at=$2
		WHERE domain_id=$1::uuid AND state='next' RETURNING `+keyCols, domainID, now))
	if err != nil {
		return Key{}, err
	}
	return k, tx.Commit(ctx)
}

// SaveResults stores the last observed state of every record and appends a run.
func (s *Store) SaveResults(ctx context.Context, domainID string, rs []Result) error {
	now := s.now()
	for _, r := range rs {
		obs, _ := json.Marshal(r.Found)
		if _, err := s.Pool.Exec(ctx, `INSERT INTO adrient.domain_dns_records
			(domain_id, kind, name, rtype, expected, required, status, observed, note, last_checked_at)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10)
			ON CONFLICT (domain_id, kind, name, rtype) DO UPDATE SET expected=$5, required=$6, status=$7,
			observed=$8::jsonb, note=$9, last_checked_at=$10`,
			domainID, r.Kind, r.Name, r.Type, r.Expected, r.Required, r.Status, string(obs), r.Message, now); err != nil {
			return err
		}
	}
	p, w, f := Summary(rs)
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.dns_check_runs (domain_id, ran_at, pass, warn, fail, clean)
		VALUES ($1::uuid,$2,$3,$4,$5,$6)`, domainID, now, p, w, f, Clean(rs))
	return err
}

// Runs returns the latest runs, newest first.
func (s *Store) Runs(ctx context.Context, domainID string) ([]Run, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ran_at, clean FROM adrient.dns_check_runs
		WHERE domain_id=$1::uuid ORDER BY ran_at DESC LIMIT 1000`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.At, &r.Clean); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Report returns the stored per-record state.
func (s *Store) Report(ctx context.Context, domainID string) ([]Result, error) {
	rows, err := s.Pool.Query(ctx, `SELECT kind, name, rtype, expected, required, status, observed, note
		FROM adrient.domain_dns_records WHERE domain_id=$1::uuid ORDER BY name, rtype`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Result
	for rows.Next() {
		var r Result
		var obs []byte
		if err := rows.Scan(&r.Kind, &r.Name, &r.Type, &r.Expected, &r.Required, &r.Status, &obs, &r.Message); err != nil {
			return nil, err
		}
		r.Value = r.Expected
		_ = json.Unmarshal(obs, &r.Found)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PlanFor builds the plan input for a domain from settings and keys.
func PlanFor(domain string, st Settings, keys []Key, webHost, defMailHost, spfInclude string) PlanInput {
	in := PlanInput{Domain: domain, MailHost: st.MailHost, WebHost: webHost, SPFIPs: st.SPFIPs, SPFInclude: spfInclude,
		DMARCPolicy: st.DMARCPolicy, DMARCRua: st.DMARCRua, MTASTSMode: st.MTASTSMode, MTASTSID: st.MTASTSID, TLSRPTRua: st.TLSRPTRua}
	if in.MailHost == "" {
		in.MailHost = defMailHost
	}
	for _, k := range keys {
		in.DKIM = append(in.DKIM, DKIMEntry{Selector: k.Selector, Algo: k.Algo, PublicKey: k.PublicKey, State: k.State})
	}
	return in
}

var _ = fmt.Sprint
