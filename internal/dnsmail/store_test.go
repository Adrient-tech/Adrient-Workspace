package dnsmail

import (
	"context"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
)

func TestStoreAndPublicEndpoints(t *testing.T) {
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var orgID, domID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO adrient.org_domains (org_id, domain, status, verify_txt, verified_at)
		VALUES ($1::uuid,'example.com','verified','x',now()) RETURNING id::text`, orgID).Scan(&domID); err != nil {
		t.Fatal(err)
	}

	clock := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	st := &Store{Pool: pool, Now: func() time.Time { return clock }}

	k1, err := st.CreateNext(ctx, "secret", domID, "")
	if err != nil || k1.Selector != "adr202611" || k1.State != StateNext {
		t.Fatalf("create: %+v %v", k1, err)
	}
	// a second generate replaces the unpublished next key and picks a new selector
	k2, err := st.CreateNext(ctx, "secret", domID, AlgoEd25519)
	if err != nil || k2.Selector != "adr202611" {
		t.Fatalf("replace next: %+v %v", k2, err)
	}
	if _, err := st.CreateNext(ctx, "", domID, ""); err == nil {
		t.Fatal("must refuse without a server secret")
	}
	act, err := st.Activate(ctx, domID)
	if err != nil || act.State != StateActive || act.ActivatedAt == nil {
		t.Fatalf("activate: %+v %v", act, err)
	}
	// rotation: new next gets a suffixed selector, activate retires the old key
	k3, err := st.CreateNext(ctx, "secret", domID, "")
	if err != nil || k3.Selector != "adr202611b" {
		t.Fatalf("rotate selector: %+v %v", k3, err)
	}
	if _, err := st.Activate(ctx, domID); err != nil {
		t.Fatal(err)
	}
	ks, _ := st.Keys(ctx, domID)
	states := map[string]string{}
	for _, k := range ks {
		states[k.Selector] = k.State
		if len(k.Private) == 0 || strings.Contains(string(k.Private), "PRIVATE") {
			t.Fatalf("private key not stored sealed: %q", k.Selector)
		}
	}
	if states["adr202611"] != StateRetired || states["adr202611b"] != StateActive {
		t.Fatalf("states %v", states)
	}

	s, err := st.PutSettings(ctx, domID, Settings{DMARCPolicy: "none", MTASTSMode: "testing", MailHost: "mail.example.com"})
	if err != nil || s.MTASTSID == "1" || s.MTASTSID == "" {
		t.Fatalf("settings %+v %v", s, err)
	}
	if _, err := st.PutSettings(ctx, domID, Settings{DMARCPolicy: "bogus", MTASTSMode: "testing"}); err == nil {
		t.Fatal("invalid settings accepted")
	}

	h := &Handler{Store: st, Secret: "secret"}
	get := func(host, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := get("mta-sts.example.com", "/.well-known/mta-sts.txt"); w.Code != 200 ||
		w.Body.String() != "version: STSv1\r\nmode: testing\r\nmx: mail.example.com\r\nmax_age: 86400\r\n" {
		t.Fatalf("mta-sts: %d %q", w.Code, w.Body.String())
	}
	if w := get("mta-sts.unknown.org", "/.well-known/mta-sts.txt"); w.Code != 404 {
		t.Fatalf("unverified domain must 404, got %d", w.Code)
	}
	if w := get("workspace.example.com", "/.well-known/mta-sts.txt"); w.Code != 404 {
		t.Fatalf("wrong host must 404, got %d", w.Code)
	}
	if w := get("autoconfig.example.com", "/mail/config-v1.1.xml"); w.Code != 200 || !strings.Contains(w.Body.String(), "<hostname>mail.example.com</hostname>") {
		t.Fatalf("autoconfig: %d %q", w.Code, w.Body.String())
	}
	if w := get("workspace.example.com", "/mail/config-v1.1.xml?emailaddress=a@example.com"); w.Code != 200 {
		t.Fatalf("autoconfig by email: %d", w.Code)
	}
	if w := get("autoconfig.example.com", "/mobileconfig?email=a@example.com"); w.Code != 200 || !strings.Contains(w.Body.String(), "a@example.com") {
		t.Fatalf("mobileconfig: %d", w.Code)
	}

	rs := Check(ctx, goodDNS(), goodFetch, testInput())
	if err := st.SaveResults(ctx, domID, rs); err != nil {
		t.Fatal(err)
	}
	back, err := st.Report(ctx, domID)
	if err != nil || len(back) != len(rs) {
		t.Fatalf("report %d/%d %v", len(back), len(rs), err)
	}
	runs, _ := st.Runs(ctx, domID)
	if len(runs) != 1 || !runs[0].Clean {
		t.Fatalf("runs %+v", runs)
	}
}
