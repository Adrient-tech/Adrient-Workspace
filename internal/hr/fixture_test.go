package hr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Adrient-tech/Adrient-Workspace/internal/testguard"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeCal struct {
	mu      sync.Mutex
	n       int
	events  map[string][4]string // id -> owner,title,start,end
	deleted []string
}

func (f *fakeCal) CreateOutOfOffice(_ context.Context, _, owner, title string, s, e time.Time, allDay bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	id := fmt.Sprintf("ev%d", f.n)
	if f.events == nil {
		f.events = map[string][4]string{}
	}
	f.events[id] = [4]string{owner, title, s.Format(time.RFC3339), e.Format(time.RFC3339)}
	return id, nil
}

func (f *fakeCal) DeleteEvent(_ context.Context, _, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.events, id)
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string // "to|title"
}

func (f *fakeNotifier) Notify(_ context.Context, _, to, _, _, title, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, to+"|"+title)
	return nil
}

func (f *fakeNotifier) has(to, titlePart string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sent {
		if strings.HasPrefix(s, to+"|") && strings.Contains(s, titlePart) {
			return true
		}
	}
	return false
}

type memBlobs struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memBlobs) Put(_ context.Context, key, _ string, _ int64, body io.Reader) error {
	data, _ := io.ReadAll(body)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.m == nil {
		b.m = map[string][]byte{}
	}
	b.m[key] = data
	return nil
}

func (b *memBlobs) Get(_ context.Context, key string) (io.ReadCloser, string, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.m[key]
	if !ok {
		return nil, "", 0, errors.New("missing blob")
	}
	return io.NopCloser(bytes.NewReader(d)), "text/plain", int64(len(d)), nil
}

func (b *memBlobs) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.m, key)
	return nil
}

type noFiles struct{}

func (noFiles) CreateHRFile(context.Context, string, string, string, string, string, int64) (string, error) {
	return "", nil
}

type fixture struct {
	t     *testing.T
	ctx   context.Context
	pool  *pgxpool.Pool
	repo  *Repository
	cal   *fakeCal
	notif *fakeNotifier
	blobs *memBlobs
	org   string
	users map[string]string // name -> user id
	emps  map[string]string // name -> employee id
	acct  *accounting.Repository
	clock *time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("ADRIENT_TEST_DSN")
	if dsn == "" {
		t.Skip("ADRIENT_TEST_DSN not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := testguard.DropSchema(ctx, t, pool); err != nil {
		t.Fatal(err)
	}
	if err := storage.RunMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, ctx: ctx, pool: pool, cal: &fakeCal{}, notif: &fakeNotifier{}, blobs: &memBlobs{}, users: map[string]string{}, emps: map[string]string{}}
	_ = pool.QueryRow(ctx, `SELECT id::text FROM adrient.orgs WHERE slug='default'`).Scan(&f.org)
	ist := time.FixedZone("IST", 5*3600+1800)
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, ist)
	f.clock = &now
	f.acct = accounting.NewRepository(pool)
	f.repo = NewRepository(pool).WithCalendar(f.cal).WithNotifier(f.notif).WithDocuments(f.blobs, noFiles{}).
		WithAccounting(AccountingAdapter{Repo: f.acct}).WithClock(func() time.Time { return *f.clock })
	if err := f.repo.EnsureSeed(ctx, f.org); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) user(name string) string {
	f.t.Helper()
	var id string
	err := f.pool.QueryRow(f.ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name) VALUES ($1,'test',$2,$3,$4) RETURNING id::text`,
		f.org, "sub-"+name, strings.ToLower(name)+"@example.test", name).Scan(&id)
	if err != nil {
		f.t.Fatal(err)
	}
	f.users[name] = id
	return id
}

func (f *fixture) actor(name string, orgAdmin bool) Actor {
	f.t.Helper()
	a, err := f.repo.ActorFor(f.ctx, f.org, f.users[name], orgAdmin)
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func (f *fixture) employee(hr Actor, name, join, manager string, withUser bool) string {
	f.t.Helper()
	e := Employee{FirstName: name, LastName: "Test", JoinDate: join, WorkEmail: strings.ToLower(name) + "@example.test", PAN: "ABCDE1234F", BankAccount: "9999" + name, BankIFSC: "HDFC0001234", Phone: "99999"}
	if withUser {
		e.UserID = f.users[name]
	}
	if manager != "" {
		e.ManagerID = f.emps[manager]
	}
	saved, err := f.repo.SaveEmployee(f.ctx, hr, "", e)
	if err != nil {
		f.t.Fatalf("save employee %s: %v", name, err)
	}
	f.emps[name] = saved.ID
	return saved.ID
}

func (f *fixture) leaveType(code string) string {
	var id string
	if err := f.pool.QueryRow(f.ctx, `SELECT id::text FROM adrient.hr_leave_types WHERE org_id=$1 AND code=$2`, f.org, code).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func wantErr(t *testing.T, what string, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s: got %v, want %v", what, err, target)
	}
}

// team: HR admin Hana, manager Meera with reports Asha and Ravi, Kabir under Hana, Nobody has no employee record.
func (f *fixture) team() (hrA Actor) {
	for _, n := range []string{"Hana", "Meera", "Asha", "Ravi", "Kabir", "Nobody"} {
		f.user(n)
	}
	hrA = f.actor("Hana", true)
	f.employee(hrA, "Hana", "2020-01-01", "", true)
	f.employee(hrA, "Meera", "2021-01-01", "", true)
	f.employee(hrA, "Asha", "2022-01-01", "Meera", true)
	f.employee(hrA, "Ravi", "2022-02-01", "Meera", true)
	f.employee(hrA, "Kabir", "2022-03-01", "Hana", true)
	return hrA
}
