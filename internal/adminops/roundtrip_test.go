package adminops

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type memStore struct {
	mu sync.Mutex
	m  map[string][]byte
	mt map[string]string
}

func newMem() *memStore { return &memStore{m: map[string][]byte{}, mt: map[string]string{}} }
func (s *memStore) List(_ context.Context, _ string, fn func(string, int64) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if err := fn(k, int64(len(v))); err != nil {
			return err
		}
	}
	return nil
}
func (s *memStore) Get(_ context.Context, k string) (io.ReadCloser, string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[k]
	if !ok {
		return nil, "", 0, fmt.Errorf("no such key %s", k)
	}
	return io.NopCloser(bytes.NewReader(v)), s.mt[k], int64(len(v)), nil
}
func (s *memStore) Put(_ context.Context, k, mime string, _ int64, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k], s.mt[k] = b, mime
	return nil
}

func scratch(t *testing.T) *pgxpool.Pool {
	t.Helper()
	admin := os.Getenv("ADRIENT_TEST_ADMIN_DSN")
	if admin == "" {
		t.Skip("ADRIENT_TEST_ADMIN_DSN not set")
	}
	ctx := context.Background()
	ap, err := pgxpool.New(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("adrient_rt_%d", time.Now().UnixNano())
	if _, err := ap.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(admin)
	u.Path = "/" + name
	p, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.Close()
		_, _ = ap.Exec(context.Background(), "DROP "+"DATABASE IF EXISTS "+name+" WITH (FORCE)")
		ap.Close()
	})
	return p
}

func rowCounts(t *testing.T, p *pgxpool.Pool) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	tabs, err := listTables(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, tb := range tabs {
		var n int64
		if err := p.QueryRow(ctx, "SELECT count(*) FROM adrient."+quoteIdent(tb)).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[tb] = n
	}
	return out
}

func seed(t *testing.T, p *pgxpool.Pool, st *memStore) (orgID string) {
	ctx := context.Background()
	must := func(q string, a ...any) {
		if _, err := p.Exec(ctx, q, a...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := p.QueryRow(ctx, `INSERT INTO adrient.orgs (slug, display_name) VALUES ('acme','Acme Ltd') RETURNING id::text`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	var uid, folder string
	if err := p.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'iss','sub1','a@acme.test','Ann ''Quoted'' Lee, "x"') RETURNING id::text`, orgID).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO adrient.drive_files (org_id, owner_id, name, mime_type) VALUES ($1,$2,'Reports','folder') RETURNING id::text`, orgID, uid).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	// Binary-ish content with newlines/commas/NUL-free bytes, to exercise CSV quoting.
	payload := []byte("line1\nline2,\"quoted\"\n\xff\xfe\x01 binary")
	st.m["orgs/acme/q1.bin"], st.mt["orgs/acme/q1.bin"] = payload, "application/octet-stream"
	must(`INSERT INTO adrient.drive_files (org_id, owner_id, parent_id, name, mime_type, storage_key, size_bytes) VALUES ($1,$2,$3,'q1.bin','application/octet-stream','orgs/acme/q1.bin',$4)`, orgID, uid, folder, len(payload))
	return orgID
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	src := scratch(t)
	if err := storage.RunMigrations(ctx, src); err != nil {
		t.Fatal(err)
	}
	srcStore := newMem()
	seed(t, src, srcStore)
	want := rowCounts(t, src)

	var buf bytes.Buffer
	m, err := Backup(ctx, src, srcStore, &buf, BackupOptions{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if m.ObjectCount != 1 {
		t.Fatalf("objects: %d", m.ObjectCount)
	}

	dst := scratch(t) // empty database, not even migrated
	dstStore := newMem()
	a, err := OpenArchive(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := Restore(ctx, dst, dstStore, a, RestoreOptions{}); err == nil {
		t.Fatal("restore without --replace must refuse")
	}
	rep, err := Restore(ctx, dst, dstStore, a, RestoreOptions{Replace: true})
	if err != nil {
		t.Fatalf("restore: %v (%v)", err, rep)
	}
	got := rowCounts(t, dst)
	var names []string
	for k := range want {
		names = append(names, k)
	}
	sort.Strings(names)
	var total int64
	for _, k := range names {
		if want[k] != got[k] {
			t.Errorf("table %s: source %d rows, restored %d", k, want[k], got[k])
		}
		total += want[k]
	}
	if !bytes.Equal(dstStore.m["orgs/acme/q1.bin"], srcStore.m["orgs/acme/q1.bin"]) || dstStore.mt["orgs/acme/q1.bin"] != "application/octet-stream" {
		t.Error("object bytes or mime differ after restore")
	}
	// A value with quotes/commas must survive.
	var dn string
	if err := dst.QueryRow(ctx, `SELECT display_name FROM adrient.users WHERE email='a@acme.test'`).Scan(&dn); err != nil || dn != `Ann 'Quoted' Lee, "x"` {
		t.Errorf("display_name round trip: %q %v", dn, err)
	}
	t.Logf("round trip ok: %d tables, %d rows, %d objects", len(names), total, rep.Objects)

	// Restoring a second time over existing data must also verify (replace semantics).
	if _, err := Restore(ctx, dst, dstStore, a, RestoreOptions{Replace: true}); err != nil {
		t.Fatalf("second restore: %v", err)
	}
}

func TestOpenArchiveRejectsTraversalAndGarbage(t *testing.T) {
	if _, err := OpenArchive(strings.NewReader("not gzip")); err == nil {
		t.Fatal("garbage accepted")
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0o600, Size: 1})
	_, _ = tw.Write([]byte("x"))
	tw.Close()
	gz.Close()
	if _, err := OpenArchive(&buf); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("traversal not rejected: %v", err)
	}
}

func TestExportOrg(t *testing.T) {
	ctx := context.Background()
	p := scratch(t)
	if err := storage.RunMigrations(ctx, p); err != nil {
		t.Fatal(err)
	}
	st := newMem()
	seed(t, p, st)
	var buf bytes.Buffer
	m, err := ExportOrg(ctx, p, st, "acme", &buf, "test")
	if err != nil {
		t.Fatal(err)
	}
	if m.Tables["users"] != 1 || m.Tables["drive_files"] != 2 || m.Files != 1 {
		t.Fatalf("manifest: %+v", m)
	}
	files := map[string][]byte{}
	gz, _ := gzip.NewReader(&buf)
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(tr)
		files[h.Name] = b
	}
	if !bytes.Equal(files["files/Reports/q1.bin"], st.m["orgs/acme/q1.bin"]) {
		t.Errorf("exported file missing/different; have %v", keys(files))
	}
	if _, ok := files["data/sessions.ndjson"]; ok {
		t.Error("sessions must not be exported")
	}
	// Other org's rows (the seeded default org) must not leak in.
	if strings.Contains(string(files["data/users.ndjson"]), "default") && !strings.Contains(string(files["data/users.ndjson"]), "acme") {
		t.Error("users export wrong org")
	}
	if len(files["data/orgs.ndjson"]) == 0 || bytes.Count(files["data/orgs.ndjson"], []byte("\n")) != 1 {
		t.Errorf("orgs.ndjson should hold exactly the one org: %q", files["data/orgs.ndjson"])
	}
	t.Logf("exported %d tables; not exported: %v", len(m.Tables), m.NotExported)
}

func keys(m map[string][]byte) []string {
	var k []string
	for n := range m {
		k = append(k, n)
	}
	sort.Strings(k)
	return k
}
