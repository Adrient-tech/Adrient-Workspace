package adminops

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Check is one doctor result. Status is "ok", "warn" or "fail".
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// DoctorOptions configures Doctor. Env is a lookup function (os.Getenv in the CLI).
type DoctorOptions struct {
	Env func(string) string
}

// Doctor runs read-only diagnostics. pool and store may be nil when they could
// not be created; that is itself reported.
func Doctor(ctx context.Context, pool *pgxpool.Pool, store ObjectStore, opt DoctorOptions) []Check {
	var out []Check
	add := func(name, status, detail string) { out = append(out, Check{name, status, detail}) }
	env := opt.Env
	if env == nil {
		env = func(string) string { return "" }
	}

	for _, k := range []string{"ADRIENT_POSTGRES_DSN", "ADRIENT_OIDC_ISSUER", "ADRIENT_OIDC_CLIENT_ID", "ADRIENT_OIDC_CLIENT_SECRET", "ADRIENT_OIDC_REDIRECT_URL", "ADRIENT_RUSTFS_ENDPOINT", "ADRIENT_RUSTFS_ACCESS_KEY", "ADRIENT_RUSTFS_SECRET_KEY"} {
		if env(k) == "" {
			add("config "+k, "fail", "not set")
		}
	}
	if env("ADRIENT_SESSION_COOKIE_SECURE") != "true" && strings.HasPrefix(env("ADRIENT_OIDC_REDIRECT_URL"), "https://") {
		add("config cookies", "warn", "redirect URL is https but ADRIENT_SESSION_COOKIE_SECURE is not true")
	}

	if pool == nil {
		add("postgres", "fail", "no connection")
		return out
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var ver string
	if err := pool.QueryRow(cctx, `SHOW server_version`).Scan(&ver); err != nil {
		add("postgres", "fail", err.Error())
		return out
	}
	add("postgres", "ok", "server "+ver)

	st, err := storage.GetMigrationStatus(cctx, pool)
	switch {
	case err != nil:
		add("migrations", "fail", err.Error())
	case len(st.Unknown) > 0:
		add("migrations", "fail", fmt.Sprintf("database is ahead of this binary (unknown versions %v); do not run this release against it", st.Unknown))
	case len(st.Pending()) > 0:
		add("migrations", "warn", fmt.Sprintf("%d pending (run: adrient-admin migrate up)", len(st.Pending())))
	default:
		add("migrations", "ok", fmt.Sprintf("%d applied, none pending", len(st.Migrations)))
	}
	dups, gaps, _ := storage.LintMigrationNames()
	if len(dups) > 0 {
		add("migration files", "fail", fmt.Sprintf("duplicate versions %v", dups))
	} else {
		add("migration files", "ok", fmt.Sprintf("no duplicates (%d unused numbers, allowed)", len(gaps)))
	}

	var orgs int
	if err := pool.QueryRow(cctx, `SELECT count(*) FROM adrient.orgs`).Scan(&orgs); err == nil {
		if orgs == 0 {
			add("organisations", "warn", "none yet; the first-run setup has not been completed")
		} else {
			add("organisations", "ok", fmt.Sprintf("%d", orgs))
		}
	}

	if store == nil {
		add("object storage", "fail", "not configured")
	} else {
		n := 0
		err := store.List(cctx, "", func(string, int64) error {
			n++
			if n >= 1 {
				return errStopList
			}
			return nil
		})
		if err != nil && err != errStopList {
			add("object storage", "fail", err.Error())
		} else {
			add("object storage", "ok", "reachable, bucket readable")
		}
	}

	if iss := env("ADRIENT_OIDC_ISSUER"); iss != "" {
		req, _ := http.NewRequestWithContext(cctx, http.MethodGet, strings.TrimRight(iss, "/")+"/.well-known/openid-configuration", nil)
		resp, err := http.DefaultClient.Do(req)
		switch {
		case err != nil:
			add("sign-in provider", "fail", err.Error())
		case resp.StatusCode != 200:
			add("sign-in provider", "fail", "discovery returned "+resp.Status)
			resp.Body.Close()
		default:
			resp.Body.Close()
			add("sign-in provider", "ok", "OIDC discovery reachable")
		}
	}
	return out
}

type stopListErr struct{}

func (stopListErr) Error() string { return "stop" }

var errStopList = stopListErr{}
