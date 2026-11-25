// Package server wires gRPC and the grpc-gateway HTTP surface together
// for the adrient-workspace backend.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Adrient-tech/Adrient-Workspace/internal/accessreview"
	"github.com/Adrient-tech/Adrient-Workspace/internal/adminroles"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auditsearch"
	"github.com/Adrient-tech/Adrient-Workspace/internal/buildinfo"
	"github.com/Adrient-tech/Adrient-Workspace/internal/globalsearch"
	"github.com/Adrient-tech/Adrient-Workspace/internal/iamops"
	"github.com/Adrient-tech/Adrient-Workspace/internal/notifyhook"
	"github.com/Adrient-tech/Adrient-Workspace/internal/offboard"
	"github.com/Adrient-tech/Adrient-Workspace/internal/scim"
	"github.com/Adrient-tech/Adrient-Workspace/internal/seats"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sso"
	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	adrientv1 "github.com/Adrient-tech/Adrient-Workspace/gen/go/adrient/v1"
	"github.com/Adrient-tech/Adrient-Workspace/internal/access"
	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/admin"
	"github.com/Adrient-tech/Adrient-Workspace/internal/adminanalytics"
	"github.com/Adrient-tech/Adrient-Workspace/internal/adminsecurity"
	"github.com/Adrient-tech/Adrient-Workspace/internal/adminusers"
	"github.com/Adrient-tech/Adrient-Workspace/internal/ai"
	"github.com/Adrient-tech/Adrient-Workspace/internal/alexa"
	"github.com/Adrient-tech/Adrient-Workspace/internal/apitokens"
	"github.com/Adrient-tech/Adrient-Workspace/internal/audit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
	"github.com/Adrient-tech/Adrient-Workspace/internal/booking"
	"github.com/Adrient-tech/Adrient-Workspace/internal/books"
	"github.com/Adrient-tech/Adrient-Workspace/internal/branding"
	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/chat"
	"github.com/Adrient-tech/Adrient-Workspace/internal/cloudimport"
	"github.com/Adrient-tech/Adrient-Workspace/internal/cloudimport/reposink"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/convert"
	"github.com/Adrient-tech/Adrient-Workspace/internal/crm"
	"github.com/Adrient-tech/Adrient-Workspace/internal/desktops"
	"github.com/Adrient-tech/Adrient-Workspace/internal/directory"
	"github.com/Adrient-tech/Adrient-Workspace/internal/dnsmail"
	"github.com/Adrient-tech/Adrient-Workspace/internal/doclinks"
	"github.com/Adrient-tech/Adrient-Workspace/internal/docs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
	"github.com/Adrient-tech/Adrient-Workspace/internal/eventmeet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/externalpolicy"
	"github.com/Adrient-tech/Adrient-Workspace/internal/forgejo"
	"github.com/Adrient-tech/Adrient-Workspace/internal/forms"
	"github.com/Adrient-tech/Adrient-Workspace/internal/gamerooms"
	"github.com/Adrient-tech/Adrient-Workspace/internal/games"
	"github.com/Adrient-tech/Adrient-Workspace/internal/geoaccess"
	"github.com/Adrient-tech/Adrient-Workspace/internal/groups"
	"github.com/Adrient-tech/Adrient-Workspace/internal/hacomponent"
	"github.com/Adrient-tech/Adrient-Workspace/internal/health"
	"github.com/Adrient-tech/Adrient-Workspace/internal/honeypot"
	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/integrationinfo"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/keep"
	"github.com/Adrient-tech/Adrient-Workspace/internal/know"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/lifecycle"
	"github.com/Adrient-tech/Adrient-Workspace/internal/live"
	"github.com/Adrient-tech/Adrient-Workspace/internal/mail"
	"github.com/Adrient-tech/Adrient-Workspace/internal/meet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/multiaccounts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/music"
	"github.com/Adrient-tech/Adrient-Workspace/internal/notifications"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgadmin"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgadminhttp"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgdomains"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgsync"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pagekit"
	pdfapp "github.com/Adrient-tech/Adrient-Workspace/internal/pdf/app"
	"github.com/Adrient-tech/Adrient-Workspace/internal/people"
	"github.com/Adrient-tech/Adrient-Workspace/internal/photos"
	"github.com/Adrient-tech/Adrient-Workspace/internal/podcasts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pos"
	"github.com/Adrient-tech/Adrient-Workspace/internal/prefs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/profile"
	"github.com/Adrient-tech/Adrient-Workspace/internal/projects"
	"github.com/Adrient-tech/Adrient-Workspace/internal/ratelimit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/search"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sheets"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sites"
	"github.com/Adrient-tech/Adrient-Workspace/internal/slides"
	"github.com/Adrient-tech/Adrient-Workspace/internal/talent"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tasks"
	"github.com/Adrient-tech/Adrient-Workspace/internal/telephony"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tickets"
	"github.com/Adrient-tech/Adrient-Workspace/internal/useravatar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
	"github.com/Adrient-tech/Adrient-Workspace/internal/versions"
	"github.com/Adrient-tech/Adrient-Workspace/internal/video"
	"github.com/Adrient-tech/Adrient-Workspace/internal/visits"
	"github.com/Adrient-tech/Adrient-Workspace/internal/vpn"
	"github.com/Adrient-tech/Adrient-Workspace/internal/whiteboards"
	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelproxy"
)

// Config bundles the runtime identity and dependencies of the server.
type Config struct {
	Version   string
	Commit    string
	StartedAt time.Time

	AuthConfig      auth.Config
	OIDC            *auth.OIDC
	Sessions        *auth.SessionStore
	UsersRepo       *users.Repository
	OrgsRepo        *orgs.Repository
	DocsRepo        *docs.Repository
	SheetsRepo      *sheets.Repository
	SlidesRepo      *slides.Repository
	ContactsRepo    *contacts.Repository
	WhiteboardsRepo *whiteboards.Repository
	CalendarRepo    *calendar.Repository
	MailRepo        *mail.Repository
	// MailBackend overrides the mail backend (e.g. the IMAP/SMTP bridge to
	// mailcow). When nil, a Postgres-backed LocalBackend over MailRepo is used.
	MailBackend mail.Backend
	// MailBlobs is the blob store for mail attachments (shared with Drive).
	MailBlobs mail.BlobStore
	// GamesRepo + GamesBlobs back the user-imported HTML games feature. When
	// both are set, the /api/v1/games upload/list/content routes are enabled.
	GamesRepo  *games.Repository
	GamesBlobs games.BlobStore
	ChatRepo   *chat.Repository
	// ChatBlobs is the blob store for chat attachments (shared with Drive/Mail).
	ChatBlobs     chat.BlobStore
	MeetRepo      *meet.Repository
	TelephonyRepo *telephony.Repository
	FormsRepo     *forms.Repository
	PhotosRepo    *photos.Repository
	// PhotosBlobs / BooksBlobs / VideoBlobs are the media blob stores (shared with Drive).
	PhotosBlobs photos.BlobStore
	BooksRepo   *books.Repository
	BooksBlobs  books.BlobStore
	VideoRepo   *video.Repository
	VideoBlobs  video.BlobStore
	// VideoShareRepo backs video sharing (per-user grants + public links).
	VideoShareRepo *video.ShareRepository
	// VideoPlaylistRepo / VideoProgressRepo / VideoCaptionRepo back the playlist,
	// watch-progress, and caption features. nil disables each feature.
	VideoPlaylistRepo *video.PlaylistRepository
	VideoProgressRepo *video.ProgressRepository
	VideoCaptionRepo  *video.CaptionRepository
	// PublicHost is adrient's public origin (e.g. https://workspace.adrient.com),
	// used to build absolute share-link URLs. Empty = relative URLs.
	PublicHost string
	MusicRepo  *music.Repository
	// MusicExtras / VideoExtras serve the redesigned Music and Video apps' JSON APIs.
	MusicExtras *music.Extras
	VideoExtras *video.Extras
	MusicBlobs music.BlobStore
	// MusicRadio is the radio recorder driving start/stop of station taps. nil
	// disables the radio control + proxy endpoints (stations still list).
	MusicRadio music.RadioController

	// PodcastsRepo backs the Podcasts app: per-user RSS subscriptions plus the
	// SSRF-guarded feed fetcher/parser. nil disables the podcasts routes.
	PodcastsRepo *podcasts.Repository
	// Stories backs the Stories app (the Podcasts tile): written stories, generated audio, library. nil disables /api/v1/stories.
	Stories *podcasts.Stories

	// ---- Alexa custom skill (self-hosted endpoint) --------------------------
	// AlexaSecret is the HMAC key used to sign the session-free music stream
	// URLs the Echo fetches. Empty disables the whole Alexa skill (the /alexa
	// endpoint and the signed stream route are not mounted).
	AlexaSecret []byte
	// AlexaPublicBaseURL is adrient's public origin (e.g. https://adrient.com) used
	// to build absolute signed stream URLs for the Echo. Falls back to the OIDC
	// redirect URL's origin when empty (wired in main.go).
	AlexaPublicBaseURL string
	// AlexaSkipVerify bypasses Alexa request-signature validation (local dev
	// only). Leave false in production so every /alexa request is verified.
	AlexaSkipVerify bool

	ProjectsRepo   *projects.Repository
	KeepRepo       *keep.Repository
	TasksRepo      *tasks.Repository
	InventoryRepo  *inventory.Repository
	PosRepo        *pos.Repository
	InvoicingRepo  *invoicing.Repository
	AccountingRepo *accounting.Repository
	CrmRepo        *crm.Repository
	TalentRepo     *talent.Repository
	// Know backs Records, Wiki, Contracts, Signatures, Legal and Cap table (internal/know).
	Know *know.Set
	// HelpMail sends email for the help desk and forms (nil when mail is not configured).
	HelpMail func(ctx context.Context, to []string, subject, htmlBody, text string) error
	// BookingRepo backs calendar appointment schedules (/api/v1/booking, /api/v1/public/booking).
	BookingRepo *booking.Repository
	// HRRepo backs the HR module (/api/v1/hr).
	HRRepo            *hr.Repository
	NotificationsRepo *notifications.Repository
	PrefsRepo         *prefs.Repository
	SitesRepo         *sites.Repository
	GroupsRepo        *groups.Repository
	SearchRepo        *search.Repository
	// AIGateway backs the AI assistant (/api/v1/ai, internal/ai). Nil disables it.
	AIGateway *ai.Gateway
	// AdminRepo backs the per-org service-enablement settings. AdminEmails is the
	// raw ADRIENT_ADMIN_EMAILS allowlist of bootstrap super-admins.
	AdminRepo   *admin.Repository
	AdminEmails string
	// AdminDefaultURLs holds deployment-default external URLs keyed by service
	// id (e.g. "homeassistant" from ADRIENT_HOMEASSISTANT_URL). Reported for any
	// org that hasn't set its own external_url for that service.
	AdminDefaultURLs map[string]string
	// OrgAdminRepo backs per-org admin roles (adrient.org_admins). It powers the
	// authorization model (allowlist OR org_admins grant), the grant/revoke API,
	// and first-admin auto-bootstrap. nil disables role-based admin (allowlist only).
	OrgAdminRepo *orgadmin.Repository
	// SharingRepo backs per-object ACL grants (adrient.object_grants) for Drive +
	// Docs: GrantAccess/RevokeAccess/ListGrants, "Shared with me", and the
	// cross-org read path. nil disables per-user sharing (link shares still work).
	SharingRepo *sharing.Repository
	// IssuerURL is the OIDC issuer stamped on adrient users' oidc_issuer; used to
	// join Zitadel user ids (oidc_subject) to adrient users for the isAdmin flag.
	// Falls back to AuthConfig.IssuerURL when empty.
	IssuerURL string
	// AuditRepo backs the cross-cutting audit trail. nil = auditing disabled.
	AuditRepo *audit.Repository
	// BrandingRepo backs per-org branding (logo + accent color) for the Admin
	// console's "Customize branding" feature and the SPA's load-time branding
	// fetch. nil disables the branding routes (the SPA then uses defaults).
	BrandingRepo *branding.Repository
	// BrandingBlobs is the blob store for uploaded org logos (shared with Drive).
	// nil disables logo upload/serving (accent color still works).
	BrandingBlobs *drive.Blobs
	// ZitadelAPIURL / ZitadelServiceToken configure the in-app security proxy to
	// the Zitadel User API v2. URL empty → falls back to AuthConfig.IssuerURL;
	// token empty → the proxy returns 503 (feature disabled).
	ZitadelAPIURL       string
	ZitadelServiceToken string

	// EmailSender is the transactional email sender (Resend). When nil (no
	// RESEND_API_KEY) invite emails are skipped but everything else still works.
	EmailSender *email.Sender
	// TXTResolver verifies domain-ownership TXT challenges (identity plan A1).
	// nil uses public resolvers; main.go swaps in a file-backed override only
	// when ADRIENT_DEV_MODE=1 (local development).
	TXTResolver orgdomains.Resolver
	DefaultOrg  orgs.Org
	Drive       *drive.Service

	// Pool is the pgxpool.Pool passed to the adminanalytics handler for
	// read-only org-scoped COUNT/SUM queries. When nil analytics returns 503.
	Pool *pgxpool.Pool

	// SlidesBlobs stores deck image assets (shared with Drive). Nil keeps
	// Slides on inline data: URLs.
	SlidesBlobs *drive.Blobs
	// DocsBlobs stores document image assets (Docs M7, shared with Drive).
	// Nil keeps Docs on inline data: URLs.
	DocsBlobs *drive.Blobs
	// AvatarRepo / AvatarBlobs back per-user avatar upload + serving.
	AvatarRepo  *useravatar.Repository
	AvatarBlobs *drive.Blobs

	// BrowserAccountStore backs multi-account switching.
	BrowserAccountStore *multiaccounts.Store
	// AccessRepo backs the published-apps registry (adrient.access_apps). nil
	// disables the /api/v1/access/* routes.
	AccessRepo *access.Repository

	// GuacURL is the public Guacamole gateway URL (ADRIENT_GUAC_URL). When set, the
	// Access page surfaces a "Browser terminal & desktop" launch button; empty
	// keeps the "coming soon" placeholder. Surfaced via GET /api/v1/access/gateway.
	GuacURL string

	// DesktopsService powers on-demand container desktops (Guacamole Phase 2),
	// constructed in main.go (adrient.com only). nil/disabled ⇒ /api/v1/desktops
	// routes 404 and no reaper runs.
	DesktopsService *desktops.Service

	// PDFFrontendURL / PDFBackendURL enable the integrated PDF (editor & sign)
	// app: requests to /pdf/* are reverse-proxied to the PDF frontend and
	// /pdf-api/* (prefix stripped) to the PDF backend, so the separate React-19
	// SPA + Go service live under adrient's single origin. Empty disables.
	PDFFrontendURL string
	PDFBackendURL  string

	// PDFBuiltin, when non-nil, mounts the PDF signing backend IN-PROCESS inside
	// adrient (gRPC services + gateway + raw-HTTP endpoints under /pdf-api/),
	// authenticated by adrient's session via the auth bridge. It is constructed
	// only when the ADRIENT_PDF_BUILTIN flag is on (see cmd/server/main.go). When
	// nil (the default), the legacy /pdf-api reverse-proxy path is used and
	// behavior is byte-for-byte identical to before this feature.
	PDFBuiltin *pdfapp.App

	// PDFStaticDir is the path to the built PDF SPA (Vite base "/pdf/"). When
	// non-empty AND PDFBuiltin is active, adrient serves the PDF frontend itself
	// from this dir for /pdf and /pdf/* (with SPA history fallback), replacing
	// the legacy PDFFrontendURL reverse-proxy. Empty leaves the reverse-proxy
	// behavior unchanged.
	PDFStaticDir string

	// OfficeConverter is the optional LibreOffice-headless converter (CC8),
	// built from ADRIENT_LIBREOFFICE* in main.go. nil or disabled ⇒
	// /api/v1/convert/capabilities reports {"enabled":false} and
	// /api/v1/convert/office returns 404.
	OfficeConverter *convert.Converter

	// LiveRepo backs multi-user live streaming. nil disables the LiveService and
	// the MediaMTX auth/ready webhooks. LiveURLs carries the public ingest/
	// playback bases used to build a stream's URLs (set from ADRIENT_LIVE_* env).
	LiveRepo *live.Repository
	LiveURLs live.URLConfig

	// LiveHLSURL / LiveWebRTCURL are the MediaMTX HLS (:8888) and WebRTC (:8889)
	// origins adrient reverse-proxies under its own origin (/live-hls, /live-webrtc)
	// so the browser uses one origin. Empty disables the respective proxy.
	LiveHLSURL    string
	LiveWebRTCURL string

	// CRMURL is the internal origin of the integrated Twenty CRM (e.g.
	// http://127.0.0.1:3000). CRMHost is the public subdomain Host that triggers
	// proxying (e.g. crm.workspace.localtest.me:8080). When a request arrives
	// with Host == CRMHost, the WHOLE request (any path) is reverse-proxied to
	// Twenty at root — Twenty owns its origin. Empty disables.
	CRMURL  string
	CRMHost string

	// BoloMpURL is the internal origin of the Orona Bolo multiplayer server
	// (e.g. http://127.0.0.1:6173). adrient reverse-proxies /bolo-mp/* to it,
	// stripping the /bolo-mp prefix (so /bolo-mp/match/<gid> -> /match/<gid>).
	// The endpoint is a WebSocket game server; Go's httputil.ReverseProxy
	// forwards the Connection: Upgrade handshake and streams the WS unbuffered.
	// Like the game-room relay it is link-gated, not account-gated, so it
	// bypasses adrient's auth middleware. Empty disables the proxy.
	BoloMpURL string

	// ForgejoURL is the internal origin of a per-instance Forgejo (git hosting),
	// e.g. http://forgejo.<ns>.svc:3000. adrient reverse-proxies /git/* to it with
	// the /git prefix stripped (so /git/user/login -> /user/login). Set Forgejo's
	// ROOT_URL to https://<host>/git/ so its generated links carry the /git prefix
	// back through this proxy. cloudflared can't strip a path prefix, but this
	// in-process proxy can — which is how the git host lives at <host>/git on the
	// same origin instead of a separate subdomain. Forgejo does its own auth, so
	// the path bypasses adrient's auth middleware. Empty disables the proxy.
	ForgejoURL string

	// ForgejoProvisioner mirrors adrient orgs/users into the per-instance Forgejo
	// (org auto-create + team membership) at /git access time. When nil or
	// unconfigured (no ADRIENT_FORGEJO_ADMIN_TOKEN) it is a no-op, which is the
	// case on prod adrient.com. See internal/forgejo.
	ForgejoProvisioner *forgejo.Provisioner

	// ForgejoWebhookSecret is the shared HMAC-SHA256 secret for verifying inbound
	// Forgejo webhook signatures (ADRIENT_FORGEJO_WEBHOOK_SECRET). Empty disables
	// the /api/v1/forgejo/webhook endpoint.
	ForgejoWebhookSecret string

	// AssembleURL is the internal origin of the Assemble spatial-collaboration
	// app, e.g. http://assemble.<ns>.svc:8080. adrient reverse-proxies /assemble/*
	// to it with the /assemble prefix stripped. Crucially this path BYPASSES
	// adrient's auth wall: Assemble runs its own auth — SSO for signed-in workspace
	// users and GUEST access for joining rooms — so a visitor hitting /assemble
	// must NOT be redirected to adrient's sign-in. Serving it on the same origin
	// lets signed-in users share the session. Empty disables the proxy.
	AssembleURL string

	// LearnURL is the internal origin of the Learn platform container (the
	// catalog-driven Learn PWA), e.g. http://learn.<ns>.svc. adrient
	// reverse-proxies /learn/* to it with the /learn prefix stripped, so Learn
	// lives under adrient's own origin at /learn instead of a separate subdomain
	// The Learn PWA uses relative asset paths + a hash router,
	// so it works unchanged under the /learn base. Public content, so this path
	// BYPASSES adrient's auth wall (no forced sign-in). Empty disables the proxy.
	LearnURL string

	// StaticDir is the path to the built React SPA. Empty disables static
	// serving (API-only mode for tests).
	StaticDir string

	// SiteName is the product name used in server-rendered link-preview
	// (Open Graph) metadata for the SPA shell — e.g. the og:title for the
	// homepage and the og:site_name everywhere. Defaults to "Adrient"; set per
	// instance via ADRIENT_SITE_NAME (e.g. "Adrient Platform" on workspace.adrient.com).
	SiteName string

	// DemoLogin configures the one-click demo login button. Disabled by default.
	DemoLogin auth.DemoConfig
	// ---- Cloud Import -------------------------------------------------------
	CloudImportRepo      *cloudimport.Repository
	CloudImportDriveRepo *drive.Repository
	CloudImportBlobs     *drive.Blobs
	// VPNHandler serves GET /api/v1/vpn/status.
	VPNHandler *vpn.Handler
}

// Server holds the gRPC server and the HTTP/REST gateway mux wrapped with middleware.
type Server struct {
	grpc        *grpc.Server
	httpHandler http.Handler
}

// New constructs a Server with all services registered.
func New(cfg Config) *Server {
	// Audit recorder resolves the caller's org/user/email from the auth context
	// (kept here so internal/audit stays free of internal/auth's gen/ dep).
	auditRec := audit.NewRecorder(cfg.AuditRepo, func(ctx context.Context) (audit.Actor, bool) {
		org, ok := auth.OrgFromContext(ctx)
		if !ok {
			return audit.Actor{}, false
		}
		a := audit.Actor{OrgID: org.ID}
		if u, ok := auth.UserFromContext(ctx); ok {
			a.UserID = u.ID
			a.Email = u.Email
		}
		return a, true
	})
	// One interceptor auto-audits every mutating RPC across all gRPC services.
	// When the PDF backend is mounted in-process (ADRIENT_PDF_BUILTIN), the PDF
	// auth bridge interceptors are CHAINED AFTER adrient's audit interceptor so the
	// PDF handlers see adrient's session identity. Off = adrient's interceptors only
	// (unchanged), and no PDF service is registered.
	//
	// RBAC: authzSvc resolves role assignments (adrient.role_assignments plus the
	// legacy org_admins grant); ADRIENT_ADMIN_EMAILS stays a break-glass admin.
	// The authz interceptor guards the Pos and Inventory RPCs (the REST gateway
	// calls those services in-process, so authz.Guard covers HTTP separately).
	var authzLoader authz.Loader = authz.AdminOnlyLoader{IsAdmin: func(ctx context.Context, orgID, userID string) (bool, error) {
		if cfg.OrgAdminRepo == nil {
			return false, nil
		}
		return cfg.OrgAdminRepo.IsAdmin(ctx, orgID, userID)
	}}
	if cfg.Pool != nil {
		authzLoader = authz.FallbackLoader{Primary: authz.PGLoader{Pool: cfg.Pool}, Fallback: authzLoader}
	}
	authzAllow := make(map[string]struct{})
	for _, e := range strings.Split(cfg.AdminEmails, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			authzAllow[e] = struct{}{}
		}
	}
	authzSvc := authz.NewService(authzLoader).WithBreakGlass(func(ctx context.Context) bool {
		u, ok := auth.UserFromContext(ctx)
		if !ok {
			return false
		}
		_, in := authzAllow[strings.ToLower(strings.TrimSpace(u.Email))]
		return in
	})
	authz.SetDefault(authzSvc)
	// App access policies (0118): who may open which app. CanOpenApp is enforced
	// by authz.Guard / driveAuthWrap (HTTP) and authz.UnaryInterceptor (gRPC).
	var authzAdmin *authz.AdminHTTP
	if cfg.Pool != nil {
		appSvc := authz.NewAppService(authz.PGAppStore{Pool: cfg.Pool}, authzSvc)
		authz.SetDefaultApps(appSvc)
		authzAdmin = &authz.AdminHTTP{Pool: cfg.Pool, Roles: authzSvc, Apps: appSvc,
			Audit: func(ctx context.Context, service, action, rtype, rid string, detail map[string]any, r *http.Request) {
				auditRec.Record(ctx, audit.Event{Service: service, Action: action, ResourceType: rtype, ResourceID: rid,
					Method: r.Method + " " + r.URL.Path, Status: "ok", Detail: detail, UserAgent: r.UserAgent()})
			}}
	} else {
		authz.SetDefaultApps(nil)
	}
	// Per-app controls (plan D) and external collaboration policy (plan C): one
	// resolver, one gate. Drive, Chat, Meet, Mail and invitations call the gate;
	// the Admin UI reads the same resolver (effective access viewer).
	var controlsH *authz.ControlsHTTP
	var extPolicyH *externalpolicy.Handler
	var collabGate *externalpolicy.Gate
	if cfg.Pool != nil {
		ctlSvc := authz.NewControlService(authz.PGControlStore{Pool: cfg.Pool}, authz.PGAppStore{Pool: cfg.Pool}, authzSvc)
		authz.SetDefaultControls(ctlSvc)
		auditCollab := func(ctx context.Context, service, action, rtype, rid string, detail map[string]any, r *http.Request) {
			auditRec.Record(ctx, audit.Event{Service: service, Action: action, ResourceType: rtype, ResourceID: rid,
				Method: r.Method + " " + r.URL.Path, Status: "ok", Detail: detail, UserAgent: r.UserAgent()})
		}
		controlsH = &authz.ControlsHTTP{Pool: cfg.Pool, Roles: authzSvc, Apps: authz.DefaultApps(), Controls: ctlSvc, Audit: auditCollab}
		extStore := &externalpolicy.Store{Pool: cfg.Pool}
		collabGate = &externalpolicy.Gate{Store: extStore, Pool: cfg.Pool}
		extPolicyH = &externalpolicy.Handler{Store: extStore, Gate: collabGate,
			Audit: func(r *http.Request, action string, detail map[string]any) {
				auditCollab(r.Context(), "external", action, "external-policy", "", detail, r)
			}}
		controlsH.OrgPolicy = collabGate.ExplainOrgPolicy
		if cfg.Drive != nil {
			cfg.Drive.WithShareGate(collabGate)
		}
	} else {
		authz.SetDefaultControls(nil)
	}
	grpcOpts := []grpc.ServerOption{grpc.ChainUnaryInterceptor(audit.NewInterceptor(auditRec), authz.UnaryInterceptor())}
	if cfg.PDFBuiltin != nil {
		grpcOpts = []grpc.ServerOption{
			grpc.ChainUnaryInterceptor(audit.NewInterceptor(auditRec), authz.UnaryInterceptor(), pdfapp.UnaryServerInterceptor()),
			grpc.ChainStreamInterceptor(pdfapp.StreamServerInterceptor()),
		}
	}
	grpcSrv := grpc.NewServer(grpcOpts...)
	if cfg.PDFBuiltin != nil {
		cfg.PDFBuiltin.RegisterGRPC(grpcSrv)
	}
	healthSvc := health.NewService(cfg.Version, cfg.Commit, cfg.StartedAt)
	adrientv1.RegisterHealthServiceServer(grpcSrv, healthSvc)

	authSvc := auth.NewService(cfg.AuthConfig, cfg.OIDC, cfg.Sessions, cfg.UsersRepo, cfg.OrgsRepo)
	// Identity (plan A/B): verified domains, invitations, user types, blocking and
	// expiry. The policy hooks the OIDC callback; the handlers are mounted below.
	var seatCheckHook func(ctx context.Context, orgID string) error
	var idDomainsH *orgdomains.Handler
	var dnsMailH *dnsmail.Handler
	var idLifecycleH *lifecycle.Handler
	if cfg.Pool != nil {
		var txt orgdomains.Resolver = orgdomains.NetResolver{}
		if cfg.TXTResolver != nil {
			txt = cfg.TXTResolver
		}
		domStore := &orgdomains.Store{Pool: cfg.Pool, Resolver: txt}
		lcStore := &lifecycle.Store{Pool: cfg.Pool}
		idPolicy := &lifecycle.Policy{Store: lcStore, Domains: domStore, Audit: auditRec.Emit}
		idPolicy.SeatCheck = func(ctx context.Context, orgID string) error {
			if seatCheckHook == nil {
				return nil
			}
			return seatCheckHook(ctx, orgID)
		}
		authSvc = authSvc.WithSignInPolicy(idPolicy)
		idDomainsH = &orgdomains.Handler{Store: domStore, Audit: auditRec.Emit}
		var dnsRes dnsmail.Resolver = &dnsmail.CachedResolver{Next: dnsmail.NetResolver{}}
		dnsFetch := dnsmail.Fetcher(dnsmail.HTTPFetcher)
		if os.Getenv("ADRIENT_DEV_MODE") == "1" && os.Getenv("ADRIENT_DEV_DNS_OVERRIDE") != "" {
			dev := dnsmail.DevResolver{Path: os.Getenv("ADRIENT_DEV_DNS_OVERRIDE"), Next: dnsmail.NetResolver{}}
			dnsRes, dnsFetch = dev, dev.Fetcher()
		}
		dnsMailH = &dnsmail.Handler{Store: &dnsmail.Store{Pool: cfg.Pool}, Domains: domStore, Resolver: dnsRes, Fetch: dnsFetch,
			Audit: auditRec.Emit, Secret: dnsmail.ServerSecret(), WebHost: os.Getenv("ADRIENT_DNS_WEB_HOST"),
			SPFInclude: os.Getenv("ADRIENT_DNS_SPF_INCLUDE")}
		idLifecycleH = &lifecycle.Handler{Store: lcStore, Policy: idPolicy, Audit: auditRec.Emit}
		idLifecycleH.InviteCheck = func(ctx context.Context, orgID, email, userType string) error {
			if collabGate == nil {
				return nil
			}
			return collabGate.CheckInvite(ctx, orgID, email, userType)
		}
		if collabGate != nil {
			idLifecycleH.GuestInviteOpen = collabGate.GuestInviteOpen
		}
		if cfg.EmailSender != nil {
			idLifecycleH.Send = cfg.EmailSender.SendInvite
		}
		// Daily-grade job (runs hourly, idempotent): block expired guests and
		// contractors, send sponsor notices, re-check domains.
		go func() {
			time.Sleep(30 * time.Second)
			t := time.NewTicker(time.Hour)
			defer t.Stop()
			for {
				if _, err := idPolicy.RunExpiry(context.Background(), nil); err != nil {
					slog.Warn("identity: expiry job", "err", err)
				}
				<-t.C
			}
		}()
	}
	if cfg.OrgAdminRepo != nil {
		// Auto-bootstrap the first member of an org with no admins as that org's
		// first admin (see internal/auth/service.go Callback + docs/rbac-design.md).
		authSvc = authSvc.WithFirstAdminBootstrapper(cfg.OrgAdminRepo)
	}
	if cfg.BrowserAccountStore != nil {
		// Multi-account: register each new login's session under the browser's
		// account list so the user can switch without a new OIDC redirect.
		authSvc = authSvc.WithBrowserAccountAdder(cfg.BrowserAccountStore)
	}
	adrientv1.RegisterAuthServiceServer(grpcSrv, authSvc)

	// orgAdminFromCtx reports whether the caller (resolved off the auth context)
	// holds an org_admins grant. Shared by the decoupled handlers' AdminCheckers.
	// Delegates to authz (Owner/Admin role or legacy org_admins row); the
	// ADRIENT_ADMIN_EMAILS break-glass is deliberately excluded here.
	orgAdminFromCtx := func(ctx context.Context) bool {
		return authzSvc.CanAssigned(ctx, authz.PermOrgAdmin)
	}

	if cfg.Drive != nil {
		adrientv1.RegisterDriveServiceServer(grpcSrv, cfg.Drive)
	}

	var docsSvc *docs.Service
	var docsHub *docs.Hub
	var docsAssets *docs.Assets
	if cfg.DocsRepo != nil {
		docsSvc = docs.NewService(cfg.DocsRepo)
		if cfg.SharingRepo != nil {
			// Per-user ACL grants for Docs (object_grants). nil notifier = no-op.
			docsSvc = docsSvc.WithSharing(cfg.SharingRepo, nil)
		}
		docsHub = docs.NewHub(cfg.DocsRepo)
		if c := docs.Capabilities(); !c.PDF {
			// Docs/Sheets build PDFs in the browser; the server path is a fallback.
			slog.Info("docs convert: no server PDF engine (client-side PDF only)", "pandoc", c.Pandoc)
		} else {
			slog.Info("docs convert: server PDF engine available", "engine", c.PDFEngine)
		}
		if collabGate != nil {
			docsSvc = docsSvc.WithLinkPolicy(collabGate.LinkVisibility)
		}
		adrientv1.RegisterDocsServiceServer(grpcSrv, docsSvc)
		if cfg.DocsBlobs != nil {
			docsAssets = newDocsAssets(cfg.DocsBlobs, cfg.DocsRepo, cfg.SharingRepo)
		}
	}

	// Version history for Sheets/Slides/Whiteboards (nil without a pool).
	versionsW := newVersionsWiring(cfg)

	var sheetsSvc *sheets.Service
	var sheetsHub *sheets.Hub
	if cfg.SheetsRepo != nil {
		sheetsSvc = sheets.NewService(cfg.SheetsRepo)
		if cfg.SharingRepo != nil {
			// Per-user ACL grants for Sheets (object_grants).
			sheetsSvc = sheetsSvc.WithSharing(cfg.SharingRepo)
		}
		sheetsHub = sheets.NewHub()
		adrientv1.RegisterSheetsServiceServer(grpcSrv, versionsW.sheetsServer(sheetsSvc))
	}

	var slidesSvc *slides.Service
	var slidesHub *slides.Hub
	var slidesAssets *slides.Assets
	var slidesMentions *slides.Mentions
	if cfg.SlidesRepo != nil {
		slidesSvc = slides.NewService(cfg.SlidesRepo)
		if cfg.SharingRepo != nil {
			// Per-user ACL grants for Slides (object_grants).
			slidesSvc = slidesSvc.WithSharing(cfg.SharingRepo)
		}
		slidesHub = slides.NewHub()
		if cfg.SlidesBlobs != nil {
			slidesAssets = newSlidesAssets(cfg.SlidesBlobs, cfg.SlidesRepo, cfg.SharingRepo)
		}
		if cfg.NotificationsRepo != nil && cfg.UsersRepo != nil {
			slidesMentions = newSlidesMentions(cfg.SlidesRepo, cfg.SharingRepo, cfg.UsersRepo, cfg.NotificationsRepo)
		}
		adrientv1.RegisterSlidesServiceServer(grpcSrv, versionsW.slidesServer(slidesSvc))
	}

	var contactsSvc *contacts.Service
	if cfg.ContactsRepo != nil {
		contactsSvc = contacts.NewService(cfg.ContactsRepo)
		adrientv1.RegisterContactsServiceServer(grpcSrv, contactsSvc)
	}

	var whiteboardsSvc *whiteboards.Service
	var whiteboardsHub *whiteboards.Hub
	if cfg.WhiteboardsRepo != nil {
		whiteboardsSvc = whiteboards.NewService(cfg.WhiteboardsRepo)
		if cfg.SharingRepo != nil {
			// Per-user ACL grants for Whiteboards (object_grants).
			whiteboardsSvc = whiteboardsSvc.WithSharing(cfg.SharingRepo)
		}
		whiteboardsHub = whiteboards.NewHub()
		adrientv1.RegisterWhiteboardsServiceServer(grpcSrv, versionsW.whiteboardsServer(whiteboardsSvc))
	}

	var calendarSvc *calendar.Service
	if cfg.CalendarRepo != nil {
		calendarSvc = calendar.NewService(cfg.CalendarRepo)
		adrientv1.RegisterCalendarServiceServer(grpcSrv, calendarSvc)
	}

	var mailSvc *mail.Service
	mailBackend := cfg.MailBackend
	if mailBackend == nil && cfg.MailRepo != nil {
		mailBackend = mail.NewLocalBackend(cfg.MailRepo)
	}
	// Wire external delivery: recipients off the workspace mail domain are sent
	// via Resend so /mail reaches outside addresses (gmail etc.). Both the local
	// Postgres backend and the Mailu IMAP bridge use the same email.Sender.
	if cfg.EmailSender != nil {
		switch be := mailBackend.(type) {
		case *mail.LocalBackend:
			be.SetExternalSender(cfg.EmailSender)
		case *mail.Bridge:
			be.SetExternalSender(cfg.EmailSender)
		}
	}
	if mailBackend != nil {
		mailSvc = mail.NewService(mailBackend, cfg.MailRepo)
		if collabGate != nil {
			mailSvc.WithGate(collabGate)
		}
		adrientv1.RegisterMailServiceServer(grpcSrv, mailSvc)
	}
	var mailExtras *mail.Extras
	if cfg.Pool != nil && cfg.MailRepo != nil {
		mailExtras = mail.NewExtras(cfg.Pool)
	}
	var mailAtt *mail.Attachments
	if cfg.MailRepo != nil && cfg.MailBlobs != nil {
		mailAtt = mail.NewAttachments(cfg.MailRepo, cfg.MailBlobs)
		if cfg.CloudImportDriveRepo != nil {
			mailAtt.SetDrive(cfg.CloudImportDriveRepo)
		}
	}

	var gamesH *games.Games
	if cfg.GamesRepo != nil && cfg.GamesBlobs != nil {
		gamesH = games.New(cfg.GamesRepo, cfg.GamesBlobs)
	}

	var chatAtt *chat.Attachments
	if cfg.ChatRepo != nil && cfg.ChatBlobs != nil {
		chatAtt = chat.NewAttachments(cfg.ChatRepo, cfg.ChatBlobs)
	}

	var docLinks *doclinks.Handler
	if cfg.Pool != nil {
		docLinks = &doclinks.Handler{Store: doclinks.NewStore(cfg.Pool, cfg.SharingRepo)}
	}
	var chatSvc *chat.Service
	var chatHub *chat.Hub
	var chatTeams *chat.TeamsHandler
	if cfg.ChatRepo != nil {
		chatHub = chat.NewHub()
		chatSvc = chat.NewService(cfg.ChatRepo, chatHub)
		if collabGate != nil {
			chatSvc.WithGate(collabGate)
		}
		// Teams-class chat API + scheduler (send-later, reminders).
		chatTeams = chat.NewTeamsHandler(chat.NewStore(cfg.ChatRepo), chatHub)
		if collabGate != nil {
			chatTeams.WithGate(collabGate)
		}
		go chatTeams.RunScheduler(context.Background(), 10*time.Second)
		adrientv1.RegisterChatServiceServer(grpcSrv, chatSvc)
		// The chat live feed is the workspace-wide push channel: every new
		// notification (any app) and every call ring reaches open tabs live.
		if nr := cfg.NotificationsRepo; nr != nil {
			hub := chatHub
			nr.OnCreate = func(n notifications.Notification) {
				hub.Publish([]string{n.UserID}, map[string]any{
					"type": "notification", "id": n.ID, "kind": n.Type,
					"title": n.Title, "body": n.Body, "url": n.TargetURL,
				})
			}
			chatTeams.SetCallNotifier(func(ctx context.Context, orgID, userID, actorID, title, body, url string) {
				_, _ = nr.Create(ctx, notifications.CreateParams{
					OrgID: orgID, UserID: userID, Type: "call", ActorUserID: actorID,
					Title: title, Body: body, TargetURL: url,
				})
			})
		}
	}

	var meetSvc *meet.Service
	var meetHub *meet.Hub
	var meetCodes *meet.CodesHandler
	var meetSFU *meet.SFUHandler
	var eventMeetHTTP *eventmeet.HTTPHandler
	if cfg.MeetRepo != nil {
		meetSvc = meet.NewService(cfg.MeetRepo)
		meetHub = meet.NewHub()
		meetSFU = meet.NewSFUHandler(meet.SFUConfigFromEnv(), cfg.MeetRepo, meetHub,
			func(r *http.Request) (string, bool) {
				org, ok := auth.OrgFromContext(r.Context())
				if !ok {
					return "", false
				}
				return org.ID, true
			},
			func(r *http.Request) (string, string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				if !ok {
					return "", "", false
				}
				name := u.DisplayName
				if name == "" {
					name = u.Email
				}
				if name == "" {
					name = u.ID
				}
				return u.ID, name, true
			},
		)
		// Meeting lifecycle: a meeting runs from the first join to the moment the
		// last participant leaves, the host ends it, or the room is closed.
		meetEnder := &meet.Finisher{Repo: cfg.MeetRepo}
		if chatHub != nil {
			meetEnder.Publish = chatHub.PublishOrg
		}
		if nr := cfg.NotificationsRepo; nr != nil {
			meetEnder.Notify = func(ctx context.Context, orgID, userID string, info meet.EndInfo) {
				_, _ = nr.Create(ctx, notifications.CreateParams{
					OrgID: orgID, UserID: userID, Type: "meet_ended", Title: "Meeting ended",
					Body:      meet.EndedBody(info),
					TargetURL: "/meet/" + info.Room.Code,
				})
			}
		}
		meetHub.OnJoin = func(roomID, userID string) { _, _ = cfg.MeetRepo.BeginSession(context.Background(), roomID, userID) }
		meetHub.OnEmpty = func(roomID string) { meetEnder.Finish(context.Background(), roomID, meet.EndedEmpty) }
		meetSFU.OnEnd = func(roomID, reason string) { meetEnder.Finish(context.Background(), roomID, reason) }
		meetSvc.OnClosing = func(ctx context.Context, roomID, reason string) { meetEnder.Finish(ctx, roomID, reason) }
		meetSFU.CanOpen = authz.CanOpenApp
		meetSFU.ExternalCheck = authz.IsExternal // only guests/externals wait; org members join directly
		if collabGate != nil {
			meetSFU.Control = func(ctx context.Context, key string) any { return authz.Control(ctx, key).Value }
			meetSFU.JoinCheck = collabGate.CheckMeetJoin
			meetSFU.AnonAllowed = collabGate.AnonymousMeetAllowed
		}
		if sc := meet.SFUConfigFromEnv(); sc.EgressEnabled && sc.S3Bucket != "" && cfg.Pool != nil && cfg.ChatBlobs != nil {
			// Recordings: egress writes to its own bucket; when one finishes the
			// backend copies it into the host's Drive "Meet recordings" folder.
			src, serr := drive.NewBlobs(context.Background(), drive.BlobsConfig{
				Endpoint: sc.S3ReadEndpoint, AccessKey: sc.S3AccessKey, SecretKey: sc.S3Secret,
				Bucket: sc.S3Bucket, Region: sc.S3Region,
			})
			if serr == nil {
				meetRepo := cfg.MeetRepo
				rec := &meet.Recorder{
					Src: src, Dst: cfg.ChatBlobs, Drive: meet.DriveAdapter{Repo: drive.NewRepository(cfg.Pool)},
					Channel: func(ctx context.Context, orgID, code string) string { return meetRepo.ChannelForCode(ctx, orgID, code) },
					Saved: func(ctx context.Context, egressID, status, fileID string) {
						_ = meetRepo.FinishRecording(ctx, egressID, status, fileID)
					},
				}
				if nr := cfg.NotificationsRepo; nr != nil {
					rec.Notify = func(ctx context.Context, orgID, userID, title, body, url string) {
						_, _ = nr.Create(ctx, notifications.CreateParams{
							OrgID: orgID, UserID: userID, Type: "meet", Title: title, Body: body, TargetURL: url,
						})
					}
				}
				if chatTeams != nil {
					rec.PostCard = chatTeams.PostAs
				}
				meetSFU.Rec = rec
				go meetSFU.ResumeRecordings(context.Background())
			}
		}
		adrientv1.RegisterMeetServiceServer(grpcSrv, meetSvc)
		meetCodes = meet.NewCodesHandler(
			cfg.MeetRepo,
			func(r *http.Request) (string, bool) {
				org, ok := auth.OrgFromContext(r.Context())
				if !ok {
					return "", false
				}
				return org.ID, true
			},
			func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				if !ok {
					return "", false
				}
				return u.ID, true
			},
		)
		if chatHub != nil {
			meetCodes.OnRestart = func(orgID, code string) {
				chatHub.PublishOrg(orgID, map[string]any{"type": "meet_restarted", "code": code})
			}
		}

		// Per-event meeting links (calendar ↔ meet), plus first-join alerts to
		// invited attendees not yet in the call. Pure HTTP + SQL — no change to
		// the protobuf calendar Event.
		if cfg.Pool != nil {
			emRepo := eventmeet.NewRepository(cfg.Pool)
			eventMeetHTTP = eventmeet.NewHTTPHandler(
				emRepo,
				func(r *http.Request) (string, bool) {
					org, ok := auth.OrgFromContext(r.Context())
					if !ok {
						return "", false
					}
					return org.ID, true
				},
				func(r *http.Request) (string, bool) {
					u, ok := auth.UserFromContext(r.Context())
					if !ok {
						return "", false
					}
					return u.ID, true
				},
			)
			if cfg.NotificationsRepo != nil {
				notifRepo := cfg.NotificationsRepo
				meetHub.OnFirstJoin = func(roomID, joinerID, joinerName string) {
					ctx := context.Background()
					info, err := emRepo.JoinNotify(ctx, roomID)
					if err != nil {
						return // room not tied to an event, or lookup failed
					}
					title := info.EventTitle
					if title == "" {
						title = "the meeting"
					}
					for _, uid := range info.TargetUserIDs {
						if uid == joinerID {
							continue
						}
						_, _ = notifRepo.Create(ctx, notifications.CreateParams{
							OrgID:       info.OrgID,
							UserID:      uid,
							Type:        "meet_join",
							ActorUserID: joinerID,
							Title:       "Meeting started",
							Body:        joinerName + " joined “" + title + "”",
							TargetURL:   "/meet/" + info.Code,
						})
					}
				}
			}
		}
	}

	var telephonySvc *telephony.Service
	var telephonyHub *telephony.Hub
	var telephonyCalls *telephony.LogCallHandler
	if cfg.TelephonyRepo != nil {
		telephonyHub = telephony.NewHub()
		telephonySvc = telephony.NewService(cfg.TelephonyRepo, telephonyHub)
		adrientv1.RegisterTelephonyServiceServer(grpcSrv, telephonySvc)
		telephonyCalls = telephony.NewLogCallHandler(
			cfg.TelephonyRepo,
			func(r *http.Request) (string, bool) {
				org, ok := auth.OrgFromContext(r.Context())
				if !ok {
					return "", false
				}
				return org.ID, true
			},
			func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				if !ok {
					return "", false
				}
				return u.ID, true
			},
		)
	}

	var formsSvc *forms.Service
	if cfg.FormsRepo != nil {
		formsSvc = forms.NewService(cfg.FormsRepo)
		adrientv1.RegisterFormsServiceServer(grpcSrv, formsSvc)
	}

	var photosSvc *photos.Service
	if cfg.PhotosRepo != nil {
		photosSvc = photos.NewService(cfg.PhotosRepo)
		adrientv1.RegisterPhotosServiceServer(grpcSrv, photosSvc)
	}
	var photosMedia *photos.Media
	if cfg.PhotosRepo != nil && cfg.PhotosBlobs != nil {
		photosMedia = photos.NewMedia(cfg.PhotosRepo, cfg.PhotosBlobs)
	}

	var booksSvc *books.Service
	var booksFiles *books.Files
	if cfg.BooksRepo != nil {
		booksSvc = books.NewService(cfg.BooksRepo)
		adrientv1.RegisterBooksServiceServer(grpcSrv, booksSvc)
		if cfg.BooksBlobs != nil {
			booksFiles = books.NewFiles(cfg.BooksRepo, cfg.BooksBlobs)
		}
	}

	var videoSvc *video.Service
	var videoHTTP *video.HTTP
	var videoCaptions *video.CaptionRepository
	if cfg.VideoRepo != nil && cfg.VideoBlobs != nil {
		videoSvc = video.NewService(cfg.VideoRepo, cfg.VideoShareRepo, cfg.VideoBlobs, cfg.PublicHost)
		if cfg.VideoPlaylistRepo != nil && cfg.VideoProgressRepo != nil && cfg.VideoCaptionRepo != nil {
			videoSvc = videoSvc.WithFeatureRepos(cfg.VideoPlaylistRepo, cfg.VideoProgressRepo, cfg.VideoCaptionRepo)
			videoCaptions = cfg.VideoCaptionRepo
		}
		videoHTTP = video.NewHTTP(cfg.VideoRepo, cfg.VideoShareRepo, cfg.VideoBlobs).WithExtras(cfg.VideoExtras)
		adrientv1.RegisterVideoServiceServer(grpcSrv, videoSvc)
	}

	var liveSvc *live.Service
	var liveWebhooks *live.Webhooks
	if cfg.LiveRepo != nil {
		liveSvc = live.NewService(cfg.LiveRepo, cfg.LiveURLs)
		liveWebhooks = live.NewWebhooks(cfg.LiveRepo)
		adrientv1.RegisterLiveServiceServer(grpcSrv, liveSvc)
	}

	var musicSvc *music.Service
	var musicHTTP *music.HTTP
	if cfg.MusicRepo != nil && cfg.MusicBlobs != nil {
		musicSvc = music.NewService(cfg.MusicRepo, cfg.MusicBlobs)
		musicHTTP = music.NewHTTP(cfg.MusicRepo, cfg.MusicBlobs).WithExtras(cfg.MusicExtras)
		if cfg.MusicRadio != nil {
			musicHTTP = musicHTTP.WithRadio(cfg.MusicRadio).
				WithRadioCacheLimits(music.RadioCacheLimitsFromEnv())
		}
		adrientv1.RegisterMusicServiceServer(grpcSrv, musicSvc)
	}

	// Podcasts: per-user RSS subscriptions + the SSRF-guarded feed proxy/parser.
	var podcastsHTTP *podcasts.HTTP
	if cfg.PodcastsRepo != nil {
		podcastsHTTP = podcasts.NewHTTP(cfg.PodcastsRepo)
	}
	// Stories: the audio-stories platform under /api/v1/stories.
	var storiesHTTP *podcasts.StoriesHTTP
	if cfg.Stories != nil {
		cfg.Stories.Audit = func(ctx context.Context, orgID, actorID, action, resourceID string, detail map[string]any) {
			auditRec.Record(ctx, audit.Event{OrgID: orgID, ActorID: actorID, Service: "podcasts", Action: action, ResourceType: "story", ResourceID: resourceID,
				Method: "event podcasts." + action, Status: "ok", Detail: detail})
		}
		storiesHTTP = &podcasts.StoriesHTTP{S: cfg.Stories, IsAdmin: func(r *http.Request) bool { return authzSvc.Can(r.Context(), authz.PermOrgAdmin) }}
	}

	// Self-hosted Alexa custom skill: plays the default org's music library on an
	// Echo. Mounted PUBLIC (before adrient's auth wall, below) — Alexa signs its own
	// requests, which the handler verifies; the Echo fetches audio from a signed,
	// session-free stream URL. Needs the music repo+blobs and a signing secret.
	var alexaHandler *alexa.Handler
	if cfg.MusicRepo != nil && cfg.MusicBlobs != nil && len(cfg.AlexaSecret) > 0 {
		base := cfg.AlexaPublicBaseURL
		if base == "" {
			base = cfg.PublicHost
		}
		alexaHandler = alexa.New(alexa.Options{
			Repo:       cfg.MusicRepo,
			Blobs:      cfg.MusicBlobs,
			OrgID:      cfg.DefaultOrg.ID,
			Secret:     cfg.AlexaSecret,
			BaseURL:    base,
			SkipVerify: cfg.AlexaSkipVerify,
		})
	}

	var projectsSvc *projects.Service
	var projectsHub *projects.Hub
	if cfg.ProjectsRepo != nil {
		projectsHub = projects.NewHub()
		projectsSvc = projects.NewService(cfg.ProjectsRepo, projectsHub)
		projectsSvc.ForgejoWebhookSecret = cfg.ForgejoWebhookSecret
		adrientv1.RegisterProjectsServiceServer(grpcSrv, projectsSvc)
	}

	var adminSvc *admin.Service
	if cfg.AdminRepo != nil {
		adminSvc = admin.NewService(cfg.AdminRepo, cfg.AdminEmails)
		var rejected []string
		adminSvc, rejected = adminSvc.WithDefaultExternalURLs(cfg.AdminDefaultURLs)
		for _, id := range rejected {
			slog.Warn("ignoring invalid default external URL (must be http(s))", "service", id)
		}
		if cfg.OrgAdminRepo != nil {
			adminSvc = adminSvc.WithAdminChecker(func(ctx context.Context, orgID, userID string) bool {
				isAdmin, err := cfg.OrgAdminRepo.IsAdmin(ctx, orgID, userID)
				return err == nil && isAdmin
			})
		}
		adrientv1.RegisterAdminServiceServer(grpcSrv, adminSvc)
	}

	var keepSvc *keep.Service
	if cfg.KeepRepo != nil {
		keepSvc = keep.NewService(cfg.KeepRepo)
		if cfg.SharingRepo != nil {
			keepSvc = keepSvc.WithSharing(cfg.SharingRepo)
		}
		adrientv1.RegisterKeepServiceServer(grpcSrv, keepSvc)
	}

	// -- Tasks --
	var tasksSvc *tasks.Service
	if cfg.TasksRepo != nil {
		tasksSvc = tasks.NewService(cfg.TasksRepo)
		adrientv1.RegisterTasksServiceServer(grpcSrv, tasksSvc)
	}
	// -- Inventory --
	var inventorySvc *inventory.Service
	if cfg.InventoryRepo != nil {
		inventorySvc = inventory.NewService(cfg.InventoryRepo)
		adrientv1.RegisterInventoryServiceServer(grpcSrv, inventorySvc)
	}
	// -- POS --
	var posSvc *pos.Service
	if cfg.PosRepo != nil {
		posSvc = pos.NewService(cfg.PosRepo)
		adrientv1.RegisterPosServiceServer(grpcSrv, posSvc)
	}
	// -- Notifications --
	var notifSvc *notifications.Service
	if cfg.NotificationsRepo != nil {
		notifSvc = notifications.NewService(cfg.NotificationsRepo)
		adrientv1.RegisterNotificationsServiceServer(grpcSrv, notifSvc)
	}

	var sitesSvc *sites.Service
	if cfg.SitesRepo != nil {
		sitesSvc = sites.NewService(cfg.SitesRepo)
		adrientv1.RegisterSitesServiceServer(grpcSrv, sitesSvc)
	}

	var groupsSvc *groups.Service
	if cfg.GroupsRepo != nil {
		groupsSvc = groups.NewService(cfg.GroupsRepo)
		adrientv1.RegisterGroupsServiceServer(grpcSrv, groupsSvc)
	}

	var searchSvc *search.Service
	if cfg.SearchRepo != nil {
		searchSvc = search.NewService(cfg.SearchRepo)
		adrientv1.RegisterSearchServiceServer(grpcSrv, searchSvc)
	}
	var prefsSvc *prefs.Service
	if cfg.PrefsRepo != nil {
		prefsSvc = prefs.NewService(cfg.PrefsRepo)
		adrientv1.RegisterPreferencesServiceServer(grpcSrv, prefsSvc)
	}

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		runtime.WithForwardResponseOption(redirectOnAuthURL),
		// Forward the "set-cookie" gRPC metadata key as the HTTP "Set-Cookie" header.
		// The auth service uses grpc.SendHeader(ctx, metadata.Pairs("set-cookie", ...))
		// to pass cookie directives through the gateway.
		runtime.WithOutgoingHeaderMatcher(func(key string) (string, bool) {
			if key == "set-cookie" {
				return "Set-Cookie", true
			}
			return runtime.DefaultHeaderMatcher(key)
		}),
	)
	_ = adrientv1.RegisterHealthServiceHandlerServer(context.Background(), mux, healthSvc)
	_ = adrientv1.RegisterAuthServiceHandlerServer(context.Background(), mux, authSvc)
	if docsSvc != nil {
		_ = adrientv1.RegisterDocsServiceHandlerServer(context.Background(), mux, docsSvc)
	}
	if sheetsSvc != nil {
		_ = adrientv1.RegisterSheetsServiceHandlerServer(context.Background(), mux, versionsW.sheetsServer(sheetsSvc))
	}
	if slidesSvc != nil {
		_ = adrientv1.RegisterSlidesServiceHandlerServer(context.Background(), mux, versionsW.slidesServer(slidesSvc))
	}
	if contactsSvc != nil {
		_ = adrientv1.RegisterContactsServiceHandlerServer(context.Background(), mux, contactsSvc)
	}
	if whiteboardsSvc != nil {
		_ = adrientv1.RegisterWhiteboardsServiceHandlerServer(context.Background(), mux, versionsW.whiteboardsServer(whiteboardsSvc))
	}
	if calendarSvc != nil {
		_ = adrientv1.RegisterCalendarServiceHandlerServer(context.Background(), mux, calendarSvc)
	}
	if mailSvc != nil {
		_ = adrientv1.RegisterMailServiceHandlerServer(context.Background(), mux, mailSvc)
	}
	if chatSvc != nil {
		_ = adrientv1.RegisterChatServiceHandlerServer(context.Background(), mux, chatSvc)
	}
	if meetSvc != nil {
		_ = adrientv1.RegisterMeetServiceHandlerServer(context.Background(), mux, meetSvc)
	}
	if telephonySvc != nil {
		_ = adrientv1.RegisterTelephonyServiceHandlerServer(context.Background(), mux, telephonySvc)
	}
	if formsSvc != nil {
		_ = adrientv1.RegisterFormsServiceHandlerServer(context.Background(), mux, formsSvc)
		_ = mux.HandlePath(http.MethodGet, forms.MyResponsePath, func(w http.ResponseWriter, r *http.Request, p map[string]string) {
			formsSvc.ServeMyResponse(w, r, p["form_id"])
		})
	}
	if photosSvc != nil {
		_ = adrientv1.RegisterPhotosServiceHandlerServer(context.Background(), mux, photosSvc)
	}
	if booksSvc != nil {
		_ = adrientv1.RegisterBooksServiceHandlerServer(context.Background(), mux, booksSvc)
	}
	if videoSvc != nil {
		_ = adrientv1.RegisterVideoServiceHandlerServer(context.Background(), mux, videoSvc)
	}
	if liveSvc != nil {
		_ = adrientv1.RegisterLiveServiceHandlerServer(context.Background(), mux, liveSvc)
	}
	if musicSvc != nil {
		_ = adrientv1.RegisterMusicServiceHandlerServer(context.Background(), mux, musicSvc)
	}
	if projectsSvc != nil {
		_ = adrientv1.RegisterProjectsServiceHandlerServer(context.Background(), mux, projectsSvc)
	}
	if adminSvc != nil {
		_ = adrientv1.RegisterAdminServiceHandlerServer(context.Background(), mux, adminSvc)
	}
	if keepSvc != nil {
		_ = adrientv1.RegisterKeepServiceHandlerServer(context.Background(), mux, keepSvc)
	}
	if tasksSvc != nil {
		_ = adrientv1.RegisterTasksServiceHandlerServer(context.Background(), mux, tasksSvc)
	}
	if inventorySvc != nil {
		_ = adrientv1.RegisterInventoryServiceHandlerServer(context.Background(), mux, inventorySvc)
	}
	if posSvc != nil {
		_ = adrientv1.RegisterPosServiceHandlerServer(context.Background(), mux, posSvc)
	}
	if notifSvc != nil {
		_ = adrientv1.RegisterNotificationsServiceHandlerServer(context.Background(), mux, notifSvc)
	}
	if sitesSvc != nil {
		_ = adrientv1.RegisterSitesServiceHandlerServer(context.Background(), mux, sitesSvc)
	}
	if groupsSvc != nil {
		_ = adrientv1.RegisterGroupsServiceHandlerServer(context.Background(), mux, groupsSvc)
	}
	if searchSvc != nil {
		_ = adrientv1.RegisterSearchServiceHandlerServer(context.Background(), mux, searchSvc)
	}
	if prefsSvc != nil {
		_ = adrientv1.RegisterPreferencesServiceHandlerServer(context.Background(), mux, prefsSvc)
	}

	if cfg.Drive != nil {
		_ = adrientv1.RegisterDriveServiceHandlerServer(context.Background(), mux, cfg.Drive)
	}

	// The collab WebSocket cannot go through the grpc-gateway mux (it does not
	// speak HTTP upgrades), so intercept it ahead of the mux. Both share the
	// auth wrapper so the handler sees the caller's user/org in context.
	apiHandler := http.Handler(mux)
	if docsHub != nil || sheetsHub != nil || slidesHub != nil || whiteboardsHub != nil || chatHub != nil || meetHub != nil || telephonyHub != nil || projectsHub != nil {
		gateway := mux
		apiHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if docsHub != nil {
				if id, ok := docsConnectID(r.URL.Path); ok {
					serveDocsWS(w, r, id, cfg.DocsRepo, cfg.SharingRepo, docsHub)
					return
				}
				if _, _, ok := docs.AssetPath(r.URL.Path); ok && docsAssets != nil {
					docsAssets.ServeHTTP(w, r)
					return
				}
				if id, ok := docsProtectionID(r.URL.Path); ok && cfg.DocsRepo != nil {
					serveDocsProtection(w, r, id, cfg.DocsRepo, docsGrantLookup(cfg.SharingRepo))
					return
				}
				if r.URL.Path == "/api/v1/docs/convert" && r.Method == http.MethodPost {
					serveDocsConvert(w, r, docsAssets)
					return
				}
				if r.URL.Path == "/api/v1/docs/convert/capabilities" && r.Method == http.MethodGet {
					serveDocsConvertCapabilities(w, r)
					return
				}
				if r.URL.Path == "/api/v1/docs/import" && r.Method == http.MethodPost {
					serveDocsImport(w, r)
					return
				}
			}
			if sheetsHub != nil {
				if id, ok := sheetsConnectID(r.URL.Path); ok {
					serveSheetsWS(w, r, id, cfg.SheetsRepo, cfg.SharingRepo, sheetsHub)
					versionsW.sessionEnded(r, versionsSheets, id)
					return
				}
				if id, ok := sheetsRecalcID(r.URL.Path); ok {
					serveSheetsRecalc(w, r, id, cfg.SheetsRepo, cfg.SharingRepo)
					return
				}
				if id, ok := sheetsStructureID(r.URL.Path); ok {
					serveSheetsStructure(w, r, id, cfg.SheetsRepo, cfg.SharingRepo)
					return
				}
				if id, ok := sheetsActionID(r.URL.Path, "deps"); ok {
					serveSheetsDeps(w, r, id, cfg.SheetsRepo, cfg.SharingRepo)
					return
				}
				if id, ok := sheetsActionID(r.URL.Path, "goalseek"); ok {
					serveSheetsGoalSeek(w, r, id, cfg.SheetsRepo, cfg.SharingRepo)
					return
				}
				// wave4/sheets: data table, scenarios, Solver-lite.
				if id, action, ok := whatIfAction(r.URL.Path); ok {
					serveSheetsWhatIf(w, r, id, action, cfg.SheetsRepo, cfg.SharingRepo)
					return
				}
			}
			if slidesHub != nil {
				if id, ok := slidesConnectID(r.URL.Path); ok {
					serveSlidesWS(w, r, id, cfg.SlidesRepo, cfg.SharingRepo, slidesHub)
					versionsW.sessionEnded(r, versionsSlides, id)
					return
				}
				if _, _, ok := slides.AssetPath(r.URL.Path); ok && slidesAssets != nil {
					slidesAssets.ServeHTTP(w, r)
					return
				}
				if _, ok := slides.MentionPath(r.URL.Path); ok && slidesMentions != nil {
					slidesMentions.ServeHTTP(w, r)
					return
				}
			}
			if whiteboardsHub != nil {
				if id, ok := whiteboardsConnectID(r.URL.Path); ok {
					serveWhiteboardsWS(w, r, id, cfg.WhiteboardsRepo, cfg.SharingRepo, whiteboardsHub)
					versionsW.sessionEnded(r, versionsWhiteboards, id)
					return
				}
			}
			if docLinks != nil && doclinks.Match(r.URL.Path) {
				docLinks.ServeHTTP(w, r)
				return
			}
			if versionsW != nil && versions.Match(r.URL.Path) {
				versionsW.handler.ServeHTTP(w, r)
				return
			}
			if chatTeams != nil {
				if chat.IsLivePath(r.URL.Path) {
					chatTeams.ServeLive(w, r)
					return
				}
				if chat.IsTeamsPath(r.URL.Path) {
					if r.Method == http.MethodGet {
						chatTeams.ServeHTTP(w, r)
					} else {
						auditRec.Log("chat", "teams", chatTeams).ServeHTTP(w, r)
					}
					return
				}
			}
			if chatHub != nil {
				if id, ok := chatConnectID(r.URL.Path); ok {
					serveChatWS(w, r, id, cfg.ChatRepo, chatHub)
					return
				}
			}
			if meetHub != nil {
				if id, ok := meet.ConnectPathID(r.URL.Path); ok {
					serveMeetWS(w, r, id, cfg.MeetRepo, meetHub)
					return
				}
			}
			if telephonyHub != nil {
				if telephony.ConnectPath(r.URL.Path) {
					serveTelephonyWS(w, r, telephonyHub)
					return
				}
			}
			if projectsHub != nil {
				if id, ok := projectsConnectID(r.URL.Path); ok {
					serveProjectsWS(w, r, id, cfg.ProjectsRepo, projectsHub)
					return
				}
			}
			gateway.ServeHTTP(w, r)
		})
	}

	// Per-user API tokens: authenticate "Authorization: Bearer grw_..." as the
	// owning user, gated to the token's scopes (enforced inside the middleware).
	apiTokensRepo := apitokens.NewRepository(cfg.Pool)
	var apiTokensHTTP *apitokens.HTTPHandler
	if apiTokensRepo != nil {
		apiTokensHTTP = apitokens.NewHTTPHandler(apiTokensRepo, apitokens.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				if !ok {
					return "", false
				}
				return u.ID, true
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				if !ok {
					return "", false
				}
				return o.ID, true
			},
			IsTokenAuth: func(r *http.Request) bool { return auth.IsTokenAuth(r.Context()) },
		})
	}

	// Org Sync: copy selected Drive files/folders + Contacts to another org the
	// caller administers. Reuses the drive repo/blobs already wired for import.
	var orgSyncHTTP *orgsync.HTTPHandler
	if orgSyncSvc := orgsync.NewService(cfg.CloudImportDriveRepo, cfg.CloudImportBlobs, cfg.ContactsRepo, cfg.OrgsRepo, cfg.OrgAdminRepo); orgSyncSvc != nil {
		orgSyncHTTP = orgsync.NewHTTPHandler(orgSyncSvc,
			func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				if !ok {
					return "", false
				}
				return o.ID, true
			},
			func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				if !ok {
					return "", false
				}
				return u.ID, true
			},
		)
	}

	// Public multiplayer game-room relay: players join a room by code (shared
	// via link) + optional password; the hub broadcasts messages between them.
	// Game-agnostic and account-free, so any game can be made multiplayer.
	gameRoomsStore := gamerooms.NewStore(cfg.Pool)
	gameRoomsHub := gamerooms.NewHub(gameRoomsStore)
	gameRoomsHTTP := gamerooms.NewHTTPHandler(gameRoomsHub)

	// Honeypot / intrusion tripwire: decoy paths + a hidden-form-field trap that
	// legitimate users never touch. Hits record an instance-global security alert
	// (adrient.honeypot_alerts, 0087). honeypotStore.Middleware wraps the router
	// early (decoy paths return an innocuous 404); the public form trap is mounted
	// at /api/v1/honeypot; the admin-gated listing is at /api/v1/admin/honeypot.
	// A nil pool yields a nil *Store, which every method tolerates (feature off).
	honeypotStore := honeypot.NewStore(cfg.Pool)

	// Visitor tracker: privacy-preserving (hashed-IP) daily distinct-visitor set
	// behind the public "N players in the last 24h" badge atop /games. A nil pool
	// yields a nil store, which every method tolerates (feature off). A background
	// pruner drops rows older than ~2 days so the table stays tiny.
	visitsStore := visits.NewStore(cfg.Pool)
	go visitsStore.StartPruner(context.Background())

	// Ticketing service (Jira-like): authenticated project/ticket management plus
	// an unauthenticated public intake surface for projects that opt into it.
	ticketsRepo := tickets.NewRepository(cfg.Pool)
	var ticketsHTTP *tickets.HTTPHandler
	var ticketsPublicHTTP *tickets.PublicHandler
	if ticketsRepo != nil {
		ticketsHTTP = tickets.NewHTTPHandler(ticketsRepo, tickets.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				if !ok {
					return "", false
				}
				return u.ID, true
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				if !ok {
					return "", false
				}
				return o.ID, true
			},
			UserName:  func(r *http.Request) string { u, _ := auth.UserFromContext(r.Context()); return u.DisplayName },
			UserEmail: func(r *http.Request) string { u, _ := auth.UserFromContext(r.Context()); return u.Email },
		})
		ticketsPublicHTTP = tickets.NewPublicHandler(ticketsRepo)
	}

	// AI assistant (/api/v1/ai/*): gateway, admin settings, Ask Adrient.
	aiHTTP := newAIHandler(cfg, auditRec)

	// CRM: auth-wrapped JSON API.
	var crmHTTP *crm.HTTPHandler
	var crmPublicHTTP *crm.PublicHandler
	if cfg.CrmRepo != nil {
		crmHTTP = crm.NewHTTPHandler(cfg.CrmRepo, crm.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
			IsAdmin: func(r *http.Request) bool { return authz.IsAdmin(r.Context()) },
		})
		crmPublicHTTP = crm.NewPublicHandler(cfg.CrmRepo)
		crmHooks := &crm.Hooks{}
		if cfg.ChatRepo != nil {
			chatRepo := cfg.ChatRepo
			crmHooks.Notify = func(ctx context.Context, org, userID, text string) error {
				ch, err := chatRepo.FindDMChannel(ctx, org, userID, userID)
				if err != nil {
					if ch, err = chatRepo.CreateChannel(ctx, org, "dm", "", []string{userID}); err != nil {
						return err
					}
				}
				_, err = chatRepo.PostMessage(ctx, ch.ID, org, userID, "CRM", text, "")
				return err
			}
		}
		if cfg.EmailSender != nil {
			sender := cfg.EmailSender
			crmHooks.SendMail = func(ctx context.Context, _ string, to []string, subject, body string) error {
				if !sender.Configured() {
					return errors.New("outgoing mail is not configured")
				}
				return sender.Send(ctx, email.Message{To: to, Subject: subject, Text: body})
			}
		}
		if mailSvc != nil {
			crmHooks.SendAsUser = func(ctx context.Context, to []string, subject, body string) (string, error) {
				m, err := mailSvc.SendMessage(ctx, &adrientv1.SendMessageRequest{ToAddrs: to, Subject: subject, Body: body})
				if err != nil {
					return "", err
				}
				return m.GetThreadId(), nil
			}
		}
		cfg.CrmRepo.SetHooks(crmHooks)
		if formsSvc != nil {
			crmRepo := cfg.CrmRepo
			formsSvc.SetOnResponse(func(ctx context.Context, orgID string, f forms.Form, resp forms.Response) {
				if err := crmRepo.FromFormsResponse(ctx, orgID, f.ID, f.Title, resp.ID, resp.RespondentEmail, resp.Answers); err != nil {
					slog.Warn("crm: forms response to lead", "err", err)
				}
			})
		}
	}

	// Booking pages: signed-in management API and the public guest API.
	var bookingHTTP *booking.HTTPHandler
	var bookingPublicHTTP *booking.PublicHandler
	if cfg.BookingRepo != nil {
		bookingHTTP = booking.NewHTTPHandler(cfg.BookingRepo, booking.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:  func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
		})
		bookingPublicHTTP = booking.NewPublicHandler(cfg.BookingRepo)
	}

	// Calendar extras: settings, find-a-time, ICS import/export, private feed, reminders.
	var calxHTTP *calendar.ExtrasHandler
	var calxFeed *calendar.FeedHandler
	if cfg.CalendarRepo != nil {
		calxHTTP = calendar.NewExtrasHandler(cfg.CalendarRepo, calendar.ExtrasAuth{
			UserID:    func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:     func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
			UserEmail: func(r *http.Request) string { u, _ := auth.UserFromContext(r.Context()); return u.Email },
		})
		calxFeed = calendar.NewFeedHandler(cfg.CalendarRepo)
		if cfg.NotificationsRepo != nil {
			nr, cr := cfg.NotificationsRepo, cfg.CalendarRepo
			go func() {
				time.Sleep(20 * time.Second)
				t := time.NewTicker(time.Minute)
				defer t.Stop()
				for {
					_, err := cr.RunReminders(context.Background(), time.Now(), func(ctx context.Context, org, user, title, body, target string) error {
						_, err := nr.Create(ctx, notifications.CreateParams{OrgID: org, UserID: user, Type: "calendar_reminder", Title: title, Body: body, TargetURL: target})
						return err
					})
					if err != nil {
						slog.Warn("calendar reminders", "err", err)
					}
					<-t.C
				}
			}()
		}
	}

	// Contacts extras: birthdays, anniversaries, undo history, links to Calendar and CRM.
	var cxHTTP *contacts.ExtrasHandler
	if cfg.ContactsRepo != nil {
		cxHTTP = contacts.NewExtrasHandler(cfg.ContactsRepo, contacts.ExtrasAuth{
			UserID: func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:  func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
		})
	}

	// Tasks extras: assignee, priority, repeat, reminders, sharing, quick add from Mail and Chat.
	var txHTTP *tasks.ExtrasHandler
	var txPublic *tasks.PublicHandler
	if cfg.TasksRepo != nil {
		txPublic = tasks.NewPublicHandler(cfg.TasksRepo)
		txHTTP = tasks.NewExtrasHandler(cfg.TasksRepo, tasks.ExtrasAuth{
			UserID: func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:  func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
		})
		if cfg.NotificationsRepo != nil {
			nr, tr := cfg.NotificationsRepo, cfg.TasksRepo
			notify := func(ctx context.Context, org, user, title, body, target string) error {
				_, err := nr.Create(ctx, notifications.CreateParams{OrgID: org, UserID: user, Type: "task", Title: title, Body: body, TargetURL: target})
				return err
			}
			txHTTP.OnAssigned = func(org, from, to, taskID, title string) {
				_, _ = nr.Create(context.Background(), notifications.CreateParams{OrgID: org, UserID: to, Type: "task_assigned", ActorUserID: from,
					Title: "Task assigned to you", Body: title, TargetURL: "/tasks"})
			}
			go func() {
				time.Sleep(25 * time.Second)
				t := time.NewTicker(time.Minute)
				defer t.Stop()
				for {
					if _, err := tr.RunReminders(context.Background(), time.Now(), notify); err != nil {
						slog.Warn("task reminders", "err", err)
					}
					<-t.C
				}
			}()
		}
	}

	// Keep extras: notebooks, image and drawing attachments, reminder delivery.
	var kxHTTP *keep.ExtrasHandler
	if cfg.KeepRepo != nil && keepSvc != nil {
		kxHTTP = keep.NewExtrasHandler(cfg.KeepRepo, keepSvc, keep.ExtrasAuth{
			UserID: func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:  func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
		})
		if cfg.NotificationsRepo != nil {
			nr, kr := cfg.NotificationsRepo, cfg.KeepRepo
			go func() {
				time.Sleep(30 * time.Second)
				t := time.NewTicker(time.Minute)
				defer t.Stop()
				for {
					_, err := kr.RunReminders(context.Background(), time.Now(), func(ctx context.Context, org, user, title, body, target string) error {
						_, err := nr.Create(ctx, notifications.CreateParams{OrgID: org, UserID: user, Type: "keep_reminder", Title: title, Body: body, TargetURL: target})
						return err
					})
					if err != nil {
						slog.Warn("keep reminders", "err", err)
					}
					<-t.C
				}
			}()
		}
	}

	// Help desk extras: SLA, rules, canned replies, customers, CSAT, help centre, reports.
	var tsHTTP *tickets.ExtrasHandler
	var tsPublic *tickets.HelpPublicHandler
	if ticketsRepo != nil {
		ticketsRepo.Deps.BaseURL = cfg.PublicHost
		if cfg.HelpMail != nil {
			ticketsRepo.Deps.Mail = cfg.HelpMail
		}
		tsHTTP = tickets.NewExtrasHandler(ticketsRepo, tickets.AuthFuncs{
			UserID:   func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:    func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
			UserName: func(r *http.Request) string { u, _ := auth.UserFromContext(r.Context()); return u.DisplayName },
			IsAdmin:  func(r *http.Request) bool { return authz.IsAdmin(r.Context()) },
			AI:       ticketsAI(cfg),
		})
		wireTicketDeps(cfg, ticketsRepo)
		go ticketsRepo.RunSweeper(context.Background(), 5*time.Minute)
		tsPublic = tickets.NewHelpPublicHandler(ticketsRepo)
	}

	// Forms extras: theme, owner notifications, send by email.
	var fxHTTP *forms.ExtrasHandler
	if cfg.FormsRepo != nil {
		fxHTTP = forms.NewExtrasHandler(cfg.FormsRepo, forms.ExtrasAuth{
			UserID:   func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:    func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
			UserName: func(r *http.Request) string { u, _ := auth.UserFromContext(r.Context()); return u.DisplayName },
		}, forms.SendDeps{Mail: cfg.HelpMail, BaseURL: cfg.PublicHost})
		if cfg.NotificationsRepo != nil {
			nr, fr := cfg.NotificationsRepo, cfg.FormsRepo
			go func() {
				time.Sleep(35 * time.Second)
				t := time.NewTicker(time.Minute)
				defer t.Stop()
				for {
					_, err := fr.RunNotifications(context.Background(), time.Now(), func(ctx context.Context, org, user, title, body, target string) error {
						_, err := nr.Create(ctx, notifications.CreateParams{OrgID: org, UserID: user, Type: "form_response", Title: title, Body: body, TargetURL: target})
						return err
					})
					if err != nil {
						slog.Warn("form notifications", "err", err)
					}
					<-t.C
				}
			}()
		}
	}

	// Projects extras: dates, dependencies, milestones, brief, saved views, templates, links.
	var pxHTTP *projects.ExtrasHandler
	if cfg.ProjectsRepo != nil {
		pxHTTP = projects.NewExtrasHandler(cfg.ProjectsRepo, projects.ExtrasAuth{
			UserID: func(r *http.Request) (string, bool) { u, ok := auth.UserFromContext(r.Context()); return u.ID, ok },
			OrgID:  func(r *http.Request) (string, bool) { o, ok := auth.OrgFromContext(r.Context()); return o.ID, ok },
		})
	}

	// Talent: auth-wrapped JSON API plus the public career page / offer accept API.
	var talentHTTP *talent.HTTPHandler
	var talentPublicHTTP *talent.PublicHandler
	if cfg.TalentRepo != nil {
		cfg.TalentRepo.Deps.Audit = func(ctx context.Context, a talent.Actor, action, rtype, rid string) {
			auditRec.Emit(ctx, "talent", action, map[string]any{"type": rtype, "id": rid})
		}
		talentHTTP = talent.NewHTTPHandler(cfg.TalentRepo, talent.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
			IsAdmin: func(r *http.Request) bool { return authz.IsAdmin(r.Context()) },
		})
		talentPublicHTTP = talent.NewPublicHandler(cfg.TalentRepo)
	}

	// Knowledge modules (records, wiki, contracts, signatures, legal, captable): one block.
	var knowHTTP []*knowkit.Handler
	wireLegalTime(cfg)
	if cfg.Know != nil {
		knowHTTP = cfg.Know.Handlers(knowkit.Auth{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
			IsAdmin: func(r *http.Request) bool { return authz.IsAdmin(r.Context()) },
		}, auditRec.Emit)
	}

	// HR: auth-wrapped JSON API. Roles are enforced inside the repository.
	var hrHTTP *hr.HTTPHandler
	if cfg.HRRepo != nil {
		hrHTTP = hr.NewHTTPHandler(cfg.HRRepo, hr.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
			IsAdmin: func(r *http.Request) bool { return authzSvc.Can(r.Context(), authz.PermOrgAdmin) },
		})
	}

	// Ops (wave 6): Shifts, Time tracking, Field service, Fleet.
	opsHTTP := buildOps(cfg, func(r *http.Request) bool { return authzSvc.Can(r.Context(), authz.PermOrgAdmin) })

	// Bridge: cross-app hand-offs (Talent -> HR, CRM deal won -> Invoicing/Projects, record links).
	var bridgeHTTP *bridgeHandler
	if cfg.Pool != nil && cfg.HRRepo != nil && cfg.TalentRepo != nil && cfg.ContactsRepo != nil && cfg.InvoicingRepo != nil && cfg.CrmRepo != nil {
		bridgeHTTP = &bridgeHandler{pool: cfg.Pool, hr: cfg.HRRepo, talent: cfg.TalentRepo, contacts: cfg.ContactsRepo,
			projects: cfg.ProjectsRepo, invoicing: cfg.InvoicingRepo, inventory: cfg.InventoryRepo, crm: cfg.CrmRepo}
		bh := bridgeHTTP
		cfg.CrmRepo.SetOnDealWon(func(ctx context.Context, org, dealID string) {
			uid := ""
			if u, ok := auth.UserFromContext(ctx); ok {
				uid = u.ID
			}
			if _, err := bh.DealWon(ctx, org, uid, dealID, false); err != nil {
				slog.Warn("crm deal won hand-off", "deal", dealID, "err", err)
			}
		})
	}

	// Invoicing: auth-wrapped JSON API plus the public share-page read.
	var invoicingHTTP *invoicing.HTTPHandler
	var invoicingPublicHTTP *invoicing.PublicHandler
	if cfg.InvoicingRepo != nil {
		var mailer invoicing.Mailer
		if cfg.EmailSender != nil {
			mailer = cfg.EmailSender
		}
		var adjust invoicing.StockAdjuster
		if cfg.InventoryRepo != nil {
			cfg.InvoicingRepo.SetStockReserver(stockReserver{repo: cfg.InventoryRepo})
			invRepo := cfg.InventoryRepo
			adjust = func(ctx context.Context, orgID, userID, productID string, qty float64, ref string) error {
				_, err := invRepo.AdjustStock(ctx, orgID, userID, productID, inventory.AdjustInput{Qty: qty, Reason: "sale", Ref: ref, Note: "Invoice " + ref})
				return err
			}
		}
		invoicingHTTP = invoicing.NewHTTPHandler(cfg.InvoicingRepo, invoicing.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
		}, mailer, adjust)
		invoicingPublicHTTP = invoicing.NewPublicHandler(cfg.InvoicingRepo)
		invoicingHTTP.StartScheduler(context.Background(), 0) // recurring invoices, subscriptions, reminders; idempotent
	}

	// POS wave-3 routes (/api/v1/pos/x/*): idempotent sales for offline mode, manager PIN, void, gift cards, Z report.
	var posExtHTTP *pos.ExtHandler
	if cfg.PosRepo != nil {
		var posMailer pos.Mailer
		if cfg.EmailSender != nil {
			posMailer = cfg.EmailSender
		}
		posExtHTTP = pos.NewExtHandler(cfg.PosRepo, pos.ExtAuth{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
		}, posMailer)
	}

	// Inventory extensions (stock, purchasing): auth-wrapped JSON API.
	var stockHTTP *inventory.HTTPHandler
	if cfg.InventoryRepo != nil {
		var stockMail inventory.Mailer
		if cfg.EmailSender != nil {
			stockMail = cfg.EmailSender
		}
		stockHTTP = inventory.NewHTTPHandler(cfg.InventoryRepo, inventory.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
			UserEmail: func(r *http.Request) string { u, _ := auth.UserFromContext(r.Context()); return u.Email },
		}, stockMail)
		if cfg.AccountingRepo != nil {
			startStockJournalSync(context.Background(), cfg.InventoryRepo, cfg.AccountingRepo)
		}
	}

	// Accounting: auth-wrapped JSON API; reads POS and Invoicing through their repositories.
	var accountingHTTP *accounting.HTTPHandler
	if cfg.AccountingRepo != nil {
		cfg.AccountingRepo.StartRecurringRunner(context.Background()) // idempotent: on start, then every 6h
		if cr := cfg.ContactsRepo; cr != nil {
			accounting.ContactSink = func(ctx context.Context, orgID, userID string, c accounting.ImportContact) (bool, error) {
				existing, err := cr.List(ctx, orgID, contacts.ListFilter{})
				if err != nil {
					return false, err
				}
				for _, e := range existing {
					if strings.EqualFold(e.DisplayName, c.Name) {
						return false, nil
					}
				}
				f := contacts.Fields{DisplayName: c.Name, Company: c.Company, Labels: []string{"Imported"}}
				if c.Email != "" {
					f.Emails = []string{c.Email}
				}
				if c.Phone != "" {
					f.Phones = []string{c.Phone}
				}
				_, err = cr.Create(ctx, orgID, userID, f)
				return err == nil, err
			}
		}
		var posSrc accounting.PosSource
		if cfg.PosRepo != nil {
			posSrc = cfg.PosRepo
		}
		var invSrc accounting.InvoiceSource
		if cfg.InvoicingRepo != nil {
			invSrc = cfg.InvoicingRepo
		}
		accountingHTTP = accounting.NewHTTPHandler(cfg.AccountingRepo, accounting.AuthFuncs{
			UserID: func(r *http.Request) (string, bool) {
				u, ok := auth.UserFromContext(r.Context())
				return u.ID, ok
			},
			UserName: func(r *http.Request) string {
				u, _ := auth.UserFromContext(r.Context())
				if u.DisplayName != "" {
					return u.DisplayName
				}
				return u.Email
			},
			OrgID: func(r *http.Request) (string, bool) {
				o, ok := auth.OrgFromContext(r.Context())
				return o.ID, ok
			},
		}, posSrc, invSrc)
	}

	// Per-IP API rate limiting (outermost), with a stricter bucket on the auth
	// endpoints to blunt credential stuffing. Tunable via ADRIENT_RATELIMIT_*.
	// The block store makes 429s observable in the admin Rate-limiting panel
	// (adrient.ratelimit_blocks, 0088); a nil pool yields a nil store (recording off).
	rateLimitStore := ratelimit.NewStore(cfg.Pool)
	rateLimiter := ratelimit.FromEnv().WithStore(rateLimitStore)
	authWrapped := rateLimiter.Middleware(
		auth.HTTPMiddleware(cfg.AuthConfig, cfg.Sessions, cfg.UsersRepo, cfg.OrgsRepo, cfg.DefaultOrg, apiTokensRepo)(authz.Guard(apiHandler)),
	)

	// Route /api/* and /healthz to the auth-wrapped gateway; everything
	// else falls through to the static SPA handler.
	versionHTTP := buildinfo.Handler(buildinfo.Resolve(cfg.Version, cfg.Commit, ""))
	static := StaticHandler(cfg.StaticDir, cfg.SiteName)
	officeConverter := cfg.OfficeConverter
	if officeConverter == nil {
		officeConverter = convert.New(convert.Config{})
	}
	officeConvertHandler := convert.Handler(officeConverter)
	driveAuthBase := auth.HTTPMiddleware(cfg.AuthConfig, cfg.Sessions, cfg.UsersRepo, cfg.OrgsRepo, cfg.DefaultOrg, apiTokensRepo)
	// Every auth-wrapped route also passes the per-app access check (authz.AppGuard).
	driveAuthWrap := func(h http.Handler) http.Handler { return driveAuthBase(authz.AdminPathGuard(authz.AppGuard(h))) }

	// Integration surface (Home Assistant `adrient` custom integration):
	// POST /api/v1/notifications/push drops a notification into the caller's
	// own feed (token scope notifications:write, 60/min/user); GET
	// /api/v1/integrations/homeassistant/info validates a URL+token pair.
	var notifPushHTTP *notifications.PushHandler
	if cfg.NotificationsRepo != nil {
		notifPushHTTP = notifications.NewPushHandler(cfg.NotificationsRepo)
	}
	haInfoHTTP := integrationinfo.Handler{Version: cfg.Version}

	// ---- Identity admin (wave 3): SSO, SCIM, seats, lifecycle, access reviews,
	// delegated admin roles, global search and the notification hook. -------------
	var (
		scimSrv     *scim.Server
		scimAdminH  *scim.Admin
		ssoH        *sso.Handler
		seatsH      *seats.Handler
		offH        *offboard.Handler
		reviewH     *accessreview.Handler
		rolesH      *adminroles.Handler
		iamH        *iamops.Handler
		notifyHook  *notifyhook.Hook
		gsearch     *globalsearch.Registry
		auditSearch *auditsearch.Handler
	)
	if cfg.Pool != nil {
		zURL := cfg.ZitadelAPIURL
		if zURL == "" {
			zURL = cfg.AuthConfig.IssuerURL
		}
		zm := zitadelmgmt.New(zURL, cfg.ZitadelServiceToken)
		identAudit := func(ctx context.Context, orgID, actorID, action string, detail map[string]any) {
			auditRec.Record(ctx, audit.Event{OrgID: orgID, ActorID: actorID, Service: "identity", Action: action, ResourceType: "admin",
				Method: "event identity." + action, Status: "ok", Detail: detail})
		}
		adminMail := func(ctx context.Context, to []string, subject, body string) error {
			if cfg.EmailSender == nil {
				return nil
			}
			return cfg.EmailSender.Send(ctx, email.Message{To: to, Subject: subject, Text: body})
		}
		offSvc := &offboard.Service{Pool: cfg.Pool, Audit: identAudit, Mail: adminMail}
		seatsSvc := &seats.Service{Pool: cfg.Pool, Apps: authz.DefaultApps(), Roles: authzSvc, Invites: &lifecycle.Store{Pool: cfg.Pool},
			Alert: offSvc.Alert, Audit: identAudit}
		if collabGate != nil {
			seatsSvc.InviteCheck = func(ctx context.Context, orgID, addr string) error {
				return collabGate.CheckInvite(ctx, orgID, addr, "")
			}
		}
		if cfg.EmailSender != nil {
			seatsSvc.SendInvite = func(ctx context.Context, to, link, orgName string) error {
				return cfg.EmailSender.SendInvite(ctx, to, "", link, orgName)
			}
		}
		offSvc.SeatCheck = seatsSvc.Check
		seatCheckHook = seatsSvc.Check
		if zm != nil {
			offSvc.IdP = zitadelIdP{zm}
		}
		// Login history, sign-in rules and session limits.
		auth.SetLoginObserver(seatsSvc.Record)
		auth.SetSessionLimits(seatsSvc.LimitsFor)
		ssoSvc := &sso.Service{Pool: cfg.Pool, ZM: zm, Audit: identAudit}
		auth.SetPasswordGate(ssoSvc.PasswordGate)
		ssoH = &sso.Handler{Svc: ssoSvc, Starter: authSvc}
		scimStore := &scim.Store{Pool: cfg.Pool}
		scimSrv = &scim.Server{Store: scimStore, SeatCheck: seatsSvc.Check, Offboard: offSvc,
			BaseURL: os.Getenv("ADRIENT_PUBLIC_URL"),
			Audit: func(ctx context.Context, orgID, action string, d map[string]any) {
				identAudit(ctx, orgID, "", action, d)
			}}
		if zm != nil {
			scimSrv.IdP = scimIdP{zm: zm, issuer: cfg.AuthConfig.IssuerURL}
		}
		scimAdminH = &scim.Admin{Store: scimStore, BaseURL: os.Getenv("ADRIENT_PUBLIC_URL"),
			Audit: func(r *http.Request, action string, d map[string]any) {
				if o, ok := auth.OrgFromContext(r.Context()); ok {
					u, _ := auth.UserFromContext(r.Context())
					identAudit(r.Context(), o.ID, u.ID, action, d)
				}
			}}
		offH = &offboard.Handler{Svc: offSvc}
		seatsH = &seats.Handler{Svc: seatsSvc, BaseURL: baseURLOf}
		reviewSvc := &accessreview.Service{Pool: cfg.Pool, Suspend: offSvc.Suspend, Alert: offSvc.Alert, Audit: identAudit,
			Notify: func(ctx context.Context, orgID, userID, title, body, url string) {
				if cfg.NotificationsRepo != nil {
					_, _ = cfg.NotificationsRepo.Create(ctx, notifications.CreateParams{OrgID: orgID, UserID: userID, Type: "access_review", Title: title, Body: body, TargetURL: url})
				}
			}}
		reviewH = &accessreview.Handler{Svc: reviewSvc}
		rolesSvc := &adminroles.Service{Pool: cfg.Pool, Tokens: apitokens.NewRepository(cfg.Pool), Roles: authzSvc, Audit: identAudit, Alert: offSvc.Alert}
		rolesH = &adminroles.Handler{Svc: rolesSvc}
		authz.DelegatedScope = rolesSvc.ScopeAllows
		// Identity operations console (wave 9): risk alerts, per-user security actions.
		iamSvc := &iamops.Service{Pool: cfg.Pool, IdP: zm, Suspend: offSvc.Suspend, Unsuspend: offSvc.Unsuspend, Audit: identAudit, Mail: adminMail,
			Notify: func(ctx context.Context, orgID, userID, title, body, url string) {
				if cfg.NotificationsRepo != nil {
					_, _ = cfg.NotificationsRepo.Create(ctx, notifications.CreateParams{OrgID: orgID, UserID: userID, Type: "security", Title: title, Body: body, TargetURL: url})
				}
			},
			Webhook: iamops.PostWebhook,
			IsAdminUser: func(ctx context.Context, orgID, userID string) bool {
				set, err := authzSvc.ResolveUser(ctx, orgID, userID)
				return err == nil && set != nil && set.Can(authz.PermOrgAdmin)
			}}
		iamH = &iamops.Handler{Svc: iamSvc,
			IsOrgAdmin: func(ctx context.Context) bool { return authzSvc.Can(ctx, authz.PermOrgAdmin) },
			IsSuper: func(ctx context.Context) bool {
				if authzSvc.Can(ctx, authz.PermOrgOwner) {
					return true
				}
				if u, ok := auth.UserFromContext(ctx); ok {
					for _, e := range strings.Split(cfg.AdminEmails, ",") {
						if strings.EqualFold(strings.TrimSpace(e), u.Email) && e != "" {
							return true
						}
					}
				}
				return false
			}}
		auth.SetLoginObserver(func(ctx context.Context, ev auth.LoginEvent) {
			seatsSvc.Record(ctx, ev)
			iamSvc.AfterRecord(ctx, ev, ev.Country, ev.City)
		})
		auth.SetSignInGate(func(ctx context.Context, ip, country string) error { return iamSvc.CheckSignIn(ctx, "", ip, country) })
		auth.SetReadOnlyCheck(iamSvc.Quarantined)
		if cfg.NotificationsRepo != nil {
			notifyHook = &notifyhook.Hook{Pool: cfg.Pool, Repo: cfg.NotificationsRepo}
		}
		gsearch = &globalsearch.Registry{}
		gsearch.RegisterBuiltins(cfg.Pool)
		if cfg.Stories != nil {
			gsearch.Register(globalsearch.Source{ID: "stories", Label: "Stories", App: "podcasts", Search: func(ctx context.Context, q globalsearch.Query) ([]globalsearch.Result, error) {
				hits, err := cfg.Stories.GlobalSearch(ctx, q.OrgID, q.UserID, q.Pattern, q.Limit)
				out := make([]globalsearch.Result, 0, len(hits))
				for _, h := range hits {
					out = append(out, globalsearch.Result{ID: h.ID, Title: h.Title, Snippet: h.Snippet, URL: h.URL})
				}
				return out, err
			}})
		}
		auditSearch = &auditsearch.Handler{Pool: cfg.Pool, OnExport: func(ctx context.Context, orgID, actorID string, rows int) {
			identAudit(ctx, orgID, actorID, "audit.export", map[string]any{"rows": rows})
		}}
		// Hourly: purge deleted accounts past their restore window; close reviews past due.
		go func() {
			time.Sleep(45 * time.Second)
			t := time.NewTicker(time.Hour)
			defer t.Stop()
			for {
				if n, err := offSvc.Purge(context.Background()); err != nil {
					slog.Warn("offboard: purge", "err", err)
				} else if n > 0 {
					slog.Info("offboard: purged deleted accounts", "count", n)
				}
				if _, err := reviewSvc.CloseDue(context.Background()); err != nil {
					slog.Warn("access review: close due", "err", err)
				}
				<-t.C
			}
		}()
	}

	// Zitadel User API v2 proxy for the in-app account-security panel. Runs
	// inside the auth middleware so the caller's oidc_subject is resolvable; the
	// proxy enforces that the path userId equals the caller's own subject.
	zitadelAPIURL := cfg.ZitadelAPIURL
	if zitadelAPIURL == "" {
		zitadelAPIURL = cfg.AuthConfig.IssuerURL
	}
	zproxy := zitadelproxy.New(zitadelAPIURL, cfg.ZitadelServiceToken,
		func(ctx context.Context) (string, bool) {
			u, ok := auth.UserFromContext(ctx)
			if !ok || u.OIDCSubject == "" {
				return "", false
			}
			return u.OIDCSubject, true
		}, nil)

	// Org user directory (member pickers): adrient's known users, plus a live
	// Zitadel search (the full org roster) when the service token is set.
	dirHandler := directory.NewHandler(cfg.UsersRepo, cfg.AuthConfig.IssuerURL, zitadelAPIURL, cfg.ZitadelServiceToken)
	if collabGate != nil {
		dirHandler.Visible = collabGate.GuestVisible
	}
	// Paged, indexed directory search for the async pickers.
	dirSearchHandler := directory.NewSearchHandler(cfg.Pool)
	if collabGate != nil {
		dirSearchHandler.Visible = collabGate.GuestVisible
	}

	// Directory: people attributes, departments, manager chain (PersonCard, org chart).
	peopleHandler := people.NewHandler(people.NewStore(cfg.Pool), people.AuthFuncs{
		OrgID: func(r *http.Request) (string, bool) {
			o, ok := auth.OrgFromContext(r.Context())
			if !ok {
				return "", false
			}
			return o.ID, true
		},
		UserID: func(r *http.Request) (string, bool) {
			u, ok := auth.UserFromContext(r.Context())
			if !ok {
				return "", false
			}
			return u.ID, true
		},
		IsAdmin: func(r *http.Request) bool { return authzSvc.Can(r.Context(), authz.PermOrgAdmin) },
	})
	if collabGate != nil {
		peopleHandler.Visible = collabGate.GuestVisible
	}

	// Self-service profile editor: the authenticated user reads/writes their own
	// identity in Zitadel (first/last name, username, phone, email) via the
	// management v1 API. Caller-only — never a supplied id.
	profileHandler := profile.NewHandler(zitadelAPIURL, cfg.ZitadelServiceToken, cfg.UsersRepo).
		WithCaller(func(ctx context.Context) (users.User, bool) {
			u, ok := auth.UserFromContext(ctx)
			return u, ok
		})

	// Admin-gated audit-log viewer (org-scoped). Same trust model as adminUsers:
	// email in ADRIENT_ADMIN_EMAILS OR an org_admins grant (no open fallback).
	auditHandler := audit.NewHandler(cfg.AuditRepo, cfg.AdminEmails).
		WithResolver(func(ctx context.Context) (string, string, bool) {
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return "", "", false
			}
			orgID := ""
			if org, ok := auth.OrgFromContext(ctx); ok {
				orgID = org.ID
			}
			return u.Email, orgID, true
		}).
		WithAdminChecker(orgAdminFromCtx)

	// Admin user management: an admin-gated (ADRIENT_ADMIN_EMAILS) proxy to the
	// Zitadel User API v2 (create/update/(de)activate/password/delete). The
	// resolver reads the caller's email off the auth context (kept here so the
	// package stays free of internal/auth's gen/ dependency).
	issuer := cfg.IssuerURL
	if issuer == "" {
		issuer = cfg.AuthConfig.IssuerURL
	}
	adminUsers := adminusers.NewHandler(cfg.AdminEmails, zitadelAPIURL, cfg.ZitadelServiceToken).
		WithResolver(func(ctx context.Context) (string, bool) {
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return "", false
			}
			return u.Email, true
		}).
		WithAdminChecker(orgAdminFromCtx).
		// isPersonal drives the SPA's hide-Admin-in-personal-orgs gate (surfaced on
		// /api/v1/admin/whoami). Reads the org off the auth context.
		WithPersonalOrgChecker(func(ctx context.Context) bool {
			org, ok := auth.OrgFromContext(ctx)
			return ok && org.IsPersonal
		})
	if cfg.OrgAdminRepo != nil {
		adminUsers = adminUsers.WithRoster(&orgAdminRoster{
			orgAdmins: cfg.OrgAdminRepo,
			users:     cfg.UsersRepo,
			issuer:    issuer,
		})
	}
	if cfg.UsersRepo != nil {
		// Scope the Users list to the caller-org's members (oidc_subjects in
		// adrient.users), and back the remove-from-org delete with a DB store that
		// drops the adrient.users row (org_admins cascades) WITHOUT touching Zitadel.
		adminUsers = adminUsers.
			WithOrgMembers(func(ctx context.Context, q string) ([]string, bool) {
				org, ok := auth.OrgFromContext(ctx)
				if !ok || org.ID == "" {
					return nil, false
				}
				ids, err := cfg.UsersRepo.ListOIDCSubjectsByOrg(ctx, org.ID, issuer, q, 200)
				if err != nil {
					return nil, false
				}
				return ids, true
			}).
			WithOrgMemberPager(func(ctx context.Context, q, cursor string, limit int) ([]string, string, bool) {
				org, ok := auth.OrgFromContext(ctx)
				if !ok || org.ID == "" {
					return nil, "", false
				}
				var key, id string
				if parts, err := pagekit.DecodeCursor(cursor); err == nil && len(parts) == 2 {
					key, id = parts[0], parts[1]
				}
				page, err := cfg.UsersRepo.ListOIDCSubjectsPage(ctx, org.ID, issuer, q, key, id, limit)
				if err != nil {
					return nil, "", false
				}
				next := ""
				if page.NextID != "" {
					next = pagekit.EncodeCursor(page.NextKey, page.NextID)
				}
				return page.Subjects, next, true
			}).
			WithMembershipStore(&orgMembershipStore{users: cfg.UsersRepo, issuer: issuer})
	}
	if cfg.EmailSender != nil {
		// Wire the Resend invite sender: when RESEND_API_KEY is set, a branded
		// invite email is dispatched after each new user is created with SendInvite=true.
		adminUsers = adminUsers.WithInviteSender(cfg.EmailSender.SendInvite)
	}

	// Bootstrap super-admin allowlist (lower-cased) shared by the org-admin HTTP
	// surface's IsAdmin closure: a caller is an admin iff their email is here OR
	// orgAdminFromCtx reports an org_admins grant. No open fallback.
	adminAllow := make(map[string]struct{})
	for _, e := range strings.Split(cfg.AdminEmails, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			adminAllow[e] = struct{}{}
		}
	}

	// isAdminFromCtx combines the allowlist + org_admins check into a single
	// predicate reused by both orgadminhttp and adminanalytics.
	isAdminFromCtx := func(ctx context.Context) bool {
		email := ""
		if u, ok := auth.UserFromContext(ctx); ok {
			email = u.Email
		}
		if _, inAllow := adminAllow[strings.ToLower(strings.TrimSpace(email))]; inAllow {
			return true
		}
		return authzSvc.Can(ctx, authz.PermOrgAdmin)
	}

	// Admin control plane for the public game-room relay: enable/disable
	// multiplayer, monitor live sessions, kick rooms/peers, view the audit log.
	// Same admin gate as audit/analytics (allowlist OR org_admins). Mounted
	// auth-wrapped under /api/v1/gamerooms/admin/* (the public WS/list relay
	// stays account-free).
	gameRoomsAdmin := gamerooms.NewAdminHandler(gameRoomsHub, gameRoomsStore, gamerooms.Identity{
		Caller: func(ctx context.Context) (string, bool) {
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return "", false
			}
			return u.Email, true
		},
		IsAdmin: isAdminFromCtx,
	})

	// Admin analytics — read-only org-scoped COUNT/SUM queries over every app
	// table. Same admin gate as audit / orgadminhttp (allowlist OR org_admins).
	analyticsHandler := adminanalytics.NewHandler(adminanalytics.Identity{
		Caller: func(ctx context.Context) (userID, email, orgID string, ok bool) {
			u, has := auth.UserFromContext(ctx)
			if !has {
				return "", "", "", false
			}
			orgID = ""
			if org, ok := auth.OrgFromContext(ctx); ok {
				orgID = org.ID
			}
			return u.ID, u.Email, orgID, true
		},
		IsAdmin: isAdminFromCtx,
	}).WithPool(cfg.Pool)
	// Surface the demo user's unique-login-IP count on the admin dashboard, but
	// only when the public demo login is actually enabled for this instance.
	if cfg.DemoLogin.Enabled && cfg.DemoLogin.Username != "" {
		analyticsHandler = analyticsHandler.WithDemoUsername(cfg.DemoLogin.Username)
	}

	// Geo-location access control — an instance-level (NOT per-org) edge policy
	// enforced against Cloudflare's CF-IPCountry header by geoMiddleware (wired
	// at the very bottom around the whole router). geoStore persists the single
	// adrient.geo_access row (0085); geoCache is a TTL-bounded snapshot the
	// middleware reads on the hot path; geoHandler is the admin-gated GET/PUT at
	// /api/v1/admin/geo, which invalidates geoCache on write (reload-on-write).
	// Same admin gate as analytics/audit (allowlist OR org_admins).
	geoStore := geoaccess.NewStore(cfg.Pool)
	geoCache := geoaccess.NewCache(geoStore)
	geoHandler := geoaccess.NewHandler(geoStore, geoCache, geoaccess.Identity{
		Caller: func(ctx context.Context) (string, bool) {
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return "", false
			}
			return u.Email, true
		},
		IsAdmin: isAdminFromCtx,
	})

	// Honeypot admin surface — recent intrusion alerts + counts + clear. Same
	// admin gate as analytics/geo (allowlist OR org_admins). Instance-global data
	// (the traps fire on unauthenticated probers, so there is no org to scope to).
	honeypotAdmin := honeypot.NewAdminHandler(honeypotStore, honeypot.Identity{
		Caller: func(ctx context.Context) (string, bool) {
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return "", false
			}
			return u.Email, true
		},
		IsAdmin: isAdminFromCtx,
	})

	// Admin rate-limiting panel — read-only observability for the per-IP API
	// limiter: effective config (ADRIENT_RATELIMIT_*), recent 429 block events, and
	// the top offending IPs. Same admin gate as analytics/honeypot. Instance-global
	// data (the limiter keys on IP, not org).
	rateLimitAdmin := ratelimit.NewAdminHandler(rateLimiter, rateLimitStore, ratelimit.Identity{
		Caller: func(ctx context.Context) (string, bool) {
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return "", false
			}
			return u.Email, true
		},
		IsAdmin: isAdminFromCtx,
	})

	// Admin security console — reads/writes the caller-org's Zitadel security
	// policies (password complexity, login/MFA/passwordless, lockout) via the
	// service PAT. Same admin gate as analytics. The Caller returns the caller's
	// Zitadel subject (oidc_subject) so the handler can resolve the org's
	// resourceOwner for x-zitadel-orgid scoping (no cross-org access).
	securityHandler := adminsecurity.NewHandler(adminsecurity.Identity{
		Caller: func(ctx context.Context) (zitadelSubject, email, adrientOrgID string, ok bool) {
			u, has := auth.UserFromContext(ctx)
			if !has {
				return "", "", "", false
			}
			adrientOrgID = ""
			if org, ok := auth.OrgFromContext(ctx); ok {
				adrientOrgID = org.ID
			}
			return u.OIDCSubject, u.Email, adrientOrgID, true
		},
		IsAdmin: isAdminFromCtx,
	}, zitadelAPIURL, cfg.ZitadelServiceToken)

	// Org settings + branding + sessions: a decoupled, admin-gated HTTP surface
	// (see internal/orgadminhttp). Identity + admin-check are resolved off the
	// auth context (allowlist OR org_admins, same gate as adminUsers/audit). The
	// store interfaces are satisfied by thin adapters so orgadminhttp stays free
	// of internal/auth's gen/ dependency. Each store is passed as a typed nil
	// interface when its underlying repo is absent, so the handler's nil-guards
	// return 503 rather than dereferencing a nil pointer.
	var orgStore orgadminhttp.OrgStore
	if cfg.OrgsRepo != nil {
		orgStore = orgStoreAdapter{cfg.OrgsRepo}
	}
	var brandingStore orgadminhttp.BrandingStore
	if cfg.BrandingRepo != nil {
		brandingStore = brandingStoreAdapter{cfg.BrandingRepo}
	}
	var brandingBlobs orgadminhttp.BlobStore
	if cfg.BrandingBlobs != nil {
		brandingBlobs = brandingBlobAdapter{cfg.BrandingBlobs}
	}
	var sessionStore orgadminhttp.SessionStore
	if cfg.Sessions != nil {
		sessionStore = sessionStoreAdapter{cfg.Sessions}
	}
	orgAdminHandler := orgadminhttp.NewHandler(
		orgadminhttp.Identity{
			Caller: func(ctx context.Context) (userID, email, orgID, sessionToken string, ok bool) {
				u, has := auth.UserFromContext(ctx)
				if !has {
					return "", "", "", "", false
				}
				if org, ok := auth.OrgFromContext(ctx); ok {
					orgID = org.ID
				}
				tok, _ := auth.SessionTokenFromContext(ctx)
				return u.ID, u.Email, orgID, tok, true
			},
			IsAdmin: isAdminFromCtx,
		},
		orgStore,
		brandingStore,
		brandingBlobs,
		sessionStore,
	)

	// User avatar upload / serve — mirrors the org branding logo pattern.
	// Caller resolver: any authenticated user.
	avatarCaller := func(ctx context.Context) (userID string, ok bool) {
		u, has := auth.UserFromContext(ctx)
		if !has {
			return "", false
		}
		return u.ID, true
	}
	var avatarBlobStore useravatar.BlobStore
	if cfg.AvatarBlobs != nil {
		avatarBlobStore = avatarBlobAdapter{cfg.AvatarBlobs}
	}
	var avatarHandler *useravatar.Handler
	if cfg.AvatarRepo != nil {
		avatarHandler = useravatar.NewHandler(avatarCaller, cfg.AvatarRepo, avatarBlobStore)
	}

	// Multi-account switching handler.
	var multiAccountHandler *multiaccounts.Handler
	if cfg.BrowserAccountStore != nil && cfg.Sessions != nil {
		maCookieCfg := multiaccounts.CookieConfig{
			Name:     cfg.AuthConfig.CookieName,
			Domain:   cfg.AuthConfig.CookieDomain,
			Secure:   cfg.AuthConfig.CookieSecure,
			Lifetime: cfg.AuthConfig.SessionLifetime,
		}
		maSessionLookup := &sessionLookupAdapter{
			sessions: cfg.Sessions,
			users:    cfg.UsersRepo,
			orgs:     cfg.OrgsRepo,
		}
		maCaller := func(ctx context.Context) multiaccounts.CallerInfo {
			u, ok := auth.UserFromContext(ctx)
			if !ok {
				return multiaccounts.CallerInfo{}
			}
			orgID := ""
			if o, ok := auth.OrgFromContext(ctx); ok {
				orgID = o.ID
			}
			tok, _ := auth.SessionTokenFromContext(ctx)
			return multiaccounts.CallerInfo{UserID: u.ID, OrgID: orgID, Token: tok, Present: true}
		}
		multiAccountHandler = multiaccounts.NewHandler(maCaller, cfg.BrowserAccountStore, maSessionLookup, maCookieCfg)
		if cfg.AvatarRepo != nil {
			multiAccountHandler = multiAccountHandler.WithAvatarCheck(func(ctx context.Context, userID string) bool {
				_, err := cfg.AvatarRepo.Get(ctx, userID)
				return err == nil
			})
		}
	}
	// Published-apps registry: a decoupled, org-scoped HTTP surface (GET any
	// member; POST/PUT/DELETE admin-gated). Runs inside the auth middleware.
	// CallerFunc resolves (userID, orgID) off the auth context (same pattern as
	// other decoupled handlers). nil repo disables the routes.
	var accessHandler *access.Handler
	if cfg.AccessRepo != nil {
		accessHandler = access.NewHandler(cfg.AccessRepo).
			WithCaller(func(ctx context.Context) (string, string, bool) {
				u, ok := auth.UserFromContext(ctx)
				if !ok {
					return "", "", false
				}
				org, ok := auth.OrgFromContext(ctx)
				if !ok {
					return u.ID, "", false
				}
				return u.ID, org.ID, true
			}).
			WithAdminChecker(orgAdminFromCtx).
			WithGatewayURL(cfg.GuacURL)
	}

	// On-demand container desktops (Guacamole Phase 2). Constructed in main.go
	// (in-cluster kube + guac clients); here we mount the HTTP handler and start
	// the idle reaper. Disabled/nil ⇒ routes 404, no reaper.
	haComponent := hacomponent.Embedded()
	var desktopsHandler *desktops.Handler
	if cfg.DesktopsService != nil && cfg.DesktopsService.Enabled() {
		desktopsHandler = desktops.NewHandler(cfg.DesktopsService).
			WithCaller(func(ctx context.Context) (desktops.User, bool) {
				u, ok := auth.UserFromContext(ctx)
				if !ok {
					return desktops.User{}, false
				}
				org, ok := auth.OrgFromContext(ctx)
				if !ok {
					return desktops.User{}, false
				}
				return desktops.User{ID: u.ID, OrgID: org.ID, Email: u.Email}, true
			})
		go cfg.DesktopsService.RunReaper(context.Background(), time.Minute)
	}

	// Integrated PDF app reverse proxies (optional). The PDF app does its own
	// OIDC auth, so these bypass adrient's auth middleware (like Drive's raw routes).
	pdfFrontend := newReverseProxy(cfg.PDFFrontendURL)               // serves /pdf/*
	pdfBackend := newStripPrefixProxy(cfg.PDFBackendURL, "/pdf-api") // serves /pdf-api/* → backend /*

	// In-process PDF backend (ADRIENT_PDF_BUILTIN). When on, /pdf-api/* is served by
	// adrient itself: the /pdf-api prefix is stripped (so the PDF handler sees
	// /api/... exactly like the standalone), adrient's auth middleware resolves the
	// session, and the HTTP bridge stamps the PDF auth context. This REPLACES the
	// reverse-proxy for /pdf-api (the proxy branch below is skipped while on).
	var pdfBuiltinHandler http.Handler
	if cfg.PDFBuiltin != nil {
		stripped := http.StripPrefix("/pdf-api", cfg.PDFBuiltin.HTTPHandler())
		// bridge (inner) runs after auth (outer) has populated the adrient user.
		pdfBuiltinHandler = driveAuthWrap(pdfapp.HTTPMiddleware(stripped))
	}

	// In-process PDF FRONTEND (Phase 2c). When the built-in PDF backend is on
	// AND a static dir is configured, adrient serves the PDF SPA itself for /pdf
	// and /pdf/* from PDFStaticDir (SPA history fallback), so no separate
	// pdf-frontend container is needed. This takes precedence over the legacy
	// PDFFrontendURL reverse-proxy below; when off (nil here), the reverse-proxy
	// branch is reached unchanged.
	var pdfStaticHandler http.Handler
	if cfg.PDFBuiltin != nil && cfg.PDFStaticDir != "" {
		pdfStaticHandler = PDFStaticHandler(cfg.PDFStaticDir)
	}

	// MediaMTX playback proxies: adrient serves HLS + WebRTC under its own origin
	// so the browser uses a single origin (and org streams are gated by adrient's
	// auth middleware — see below). Prefix is stripped so /live-hls/<path>/...
	// → MediaMTX /<path>/...
	liveHLS := newStripPrefixProxy(cfg.LiveHLSURL, "/live-hls")          // /live-hls/*    → :8888/*
	liveWebRTC := newStripPrefixProxy(cfg.LiveWebRTCURL, "/live-webrtc") // /live-webrtc/* → :8889/*

	// Integrated Twenty CRM reverse proxy (optional). Routed by Host, not path:
	// any request whose Host is the CRM subdomain is proxied whole to Twenty at
	// root. Go's httputil.ReverseProxy forwards Connection: Upgrade (WebSockets,
	// used by Twenty's GraphQL subscriptions) and streams bodies unbuffered, so
	// uploads pass through. Twenty does its own OIDC, so this bypasses adrient's
	// auth middleware (like the PDF proxies).
	crmProxy := newReverseProxy(cfg.CRMURL)

	// Orona Bolo multiplayer (authoritative WS game server) reverse proxy.
	// /bolo-mp/* → the Bolo MP server with the /bolo-mp prefix stripped, so the
	// browser opens wss://<adrient-host>/bolo-mp/match/<gid> and it reaches the
	// server's /match/<gid>. httputil.ReverseProxy forwards the WebSocket
	// Upgrade handshake and switches to unbuffered byte streaming on the 101,
	// so the 50Hz sim deltas pass through. Public/link-gated (no adrient account).
	boloMpProxy := newStripPrefixProxy(cfg.BoloMpURL, "/bolo-mp")

	// /git/* → a per-instance Forgejo with the /git prefix stripped. Forgejo
	// serves at its root and ROOT_URL=https://<host>/git/ makes its links carry
	// the /git prefix; this proxy strips it on the way in (cloudflared can't), so
	// the git host lives at <host>/git on the same origin. Forgejo does its own
	// auth, so this bypasses adrient's auth wall.
	forgejoProxy := newForgejoProxy(cfg.ForgejoURL, "/git", cfg.ForgejoProvisioner, isAdminFromCtx)
	// forgejoAuthWrap resolves the adrient session (cookie or bearer) into the
	// request context WITHOUT requiring it — anonymous visitors pass through with
	// no user, so public Forgejo browsing still works. The proxy director then
	// reads the (optional) user/org from context to drive reverse-proxy SSO.
	forgejoAuthWrap := auth.HTTPMiddleware(cfg.AuthConfig, cfg.Sessions, cfg.UsersRepo, cfg.OrgsRepo, cfg.DefaultOrg, apiTokensRepo)

	// /assemble/* → the Assemble spatial-collaboration app, /assemble prefix
	// stripped. This BYPASSES adrient's auth wall on purpose: Assemble does its own
	// auth (workspace SSO + guest room access), so visitors must reach it without
	// being bounced to adrient's sign-in. Same origin = signed-in users share the
	// session. Configure Assemble's base path to /assemble.
	assembleProxy := newStripPrefixProxy(cfg.AssembleURL, "/assemble")

	// /learn/* → the Learn platform container (Learn PWA), /learn prefix
	// stripped. Public content; bypasses adrient's auth wall like Assemble.
	learnProxy := newStripPrefixProxy(cfg.LearnURL, "/learn")

	// ---- Cloud Import (plain HTTP, no gRPC) ----------------------------------
	// Wire concrete app-repo closures into cloudimport via the injected-interface
	// pattern so cloudimport has no import-time dependency on internal/calendar,
	// internal/contacts, or internal/drive (avoids import cycles and keeps it
	// standalone-testable).
	var cloudImportHandler *cloudimport.Handler
	if cfg.CloudImportRepo != nil {
		var contactsImp cloudimport.ContactImporter
		if cfg.ContactsRepo != nil {
			contactsImp = cloudimport.NewContactImporter(func(ctx context.Context, orgID, userID string, f cloudimport.ContactFields) error {
				_, err := cfg.ContactsRepo.Create(ctx, orgID, userID, contacts.Fields{
					DisplayName: f.DisplayName,
					FirstName:   f.FirstName,
					LastName:    f.LastName,
					Company:     f.Company,
					JobTitle:    f.JobTitle,
					Emails:      f.Emails,
					Phones:      f.Phones,
					Labels:      f.Labels,
					Notes:       f.Notes,
				})
				return err
			})
		}
		var calendarImp cloudimport.EventImporter
		if cfg.CalendarRepo != nil {
			calendarImp = cloudimport.NewEventImporter(func(ctx context.Context, orgID, userID string, f cloudimport.EventFields) error {
				_, err := cfg.CalendarRepo.Create(ctx, orgID, userID, calendar.Fields{
					Title:       f.Title,
					Description: f.Description,
					Location:    f.Location,
					StartAt:     f.StartAt,
					EndAt:       f.EndAt,
					AllDay:      f.AllDay,
					Recurrence:  f.Recurrence,
				})
				return err
			})
		}
		var driveImp cloudimport.FileImporter
		if cfg.CloudImportDriveRepo != nil && cfg.CloudImportBlobs != nil {
			driveRepo := cfg.CloudImportDriveRepo
			driveBlobs := cfg.CloudImportBlobs
			driveImp = cloudimport.NewFileImporter(func(ctx context.Context, orgID, userID, parent, name, mimeType string, size int64, r io.Reader) error {
				key, err := drive.NewStorageKey()
				if err != nil {
					return err
				}
				if err := driveBlobs.Put(ctx, key, mimeType, size, r); err != nil {
					return err
				}
				if _, err := driveRepo.CreateFile(ctx, orgID, userID, parent, name, mimeType, key, size); err != nil {
					_ = driveBlobs.Delete(ctx, key)
					return err
				}
				return nil
			})
		}
		orch := cloudimport.NewOrchestrator(cfg.CloudImportRepo, contactsImp, calendarImp, driveImp)
		if cfg.MailRepo != nil { // wave3/migrate: Takeout .mbox import
			mailSink := &reposink.Sink{Pool: cfg.Pool, Mail: cfg.MailRepo}
			orch.WithMail(func(ctx context.Context, orgID, userID string, m cloudimport.MailFields) error {
				_, err := mailSink.AddMail(ctx, orgID, userID, m)
				return err
			})
		}
		cloudImportHandler = cloudimport.NewHandler(
			cfg.CloudImportRepo,
			orch,
			func(ctx context.Context) (orgID, userID string, ok bool) {
				u, uok := auth.UserFromContext(ctx)
				o, ook := auth.OrgFromContext(ctx)
				if !uok || !ook {
					return "", "", false
				}
				return o.ID, u.ID, true
			},
		)
		// ---- Migration wizard (wave3/migrate) ----
		cloudImportHandler.Wizard = &cloudimport.Wizard{
			Store: cloudimport.NewWizardStore(cfg.Pool, dnsmail.ServerSecret()),
			Sink: &reposink.Sink{Pool: cfg.Pool, Drive: cfg.CloudImportDriveRepo, Blobs: cfg.CloudImportBlobs, Sharing: cfg.SharingRepo,
				Contacts: cfg.ContactsRepo, Calendar: cfg.CalendarRepo, Mail: cfg.MailRepo, Groups: cfg.GroupsRepo, Chat: cfg.ChatRepo},
			CallerOf: func(ctx context.Context) (string, string, string, bool) {
				u, uok := auth.UserFromContext(ctx)
				o, ook := auth.OrgFromContext(ctx)
				return o.ID, u.ID, u.Email, uok && ook
			},
			IsAdmin:           func(r *http.Request) bool { return authzSvc.Can(r.Context(), authz.PermOrgAdmin) },
			Audit:             auditRec.Emit,
			RedirectURL:       cloudimport.CallbackURL(cfg.AuthConfig.RedirectURL),
			AllowPrivateHosts: os.Getenv("ADRIENT_IMPORT_ALLOW_PRIVATE_HOSTS") == "1",
		}
		cloudImportHandler.Wizard.ResumeAfterRestart(context.Background())
	}
	// In-app password login and one-click demo login. These are public (no
	// auth wrapper required — they ARE the sign-in mechanism). They talk to
	// Zitadel server-side via the service token, mint a adrient session, and set
	// the session cookie directly.
	var zitadelClient *auth.ZitadelClient
	if cfg.ZitadelServiceToken != "" {
		apiURL := cfg.ZitadelAPIURL
		if apiURL == "" {
			apiURL = cfg.AuthConfig.IssuerURL
		}
		zitadelClient = auth.NewZitadelClient(apiURL, cfg.ZitadelServiceToken)
	}
	loginHandlers := auth.NewLoginHandlers(
		cfg.AuthConfig,
		zitadelClient,
		cfg.Sessions,
		cfg.UsersRepo,
		cfg.OrgsRepo,
		issuer,
	).WithDemoLoginAudit(func(r *http.Request, orgID, userID, email string) {
		// Demo sign-in is pre-auth (no session on the inbound request), so we pass
		// the resolved demo identity explicitly. Best-effort; a nil recorder or an
		// empty org is dropped by the recorder.
		auditRec.Record(r.Context(), audit.Event{
			OrgID:        orgID,
			ActorID:      userID,
			ActorEmail:   email,
			Service:      "auth",
			Action:       "demo-login",
			ResourceType: "session",
			Method:       r.Method + " " + r.URL.Path,
			Status:       "ok",
			IP:           r.Header.Get("X-Forwarded-For"),
			UserAgent:    r.UserAgent(),
		})
	})

	// auditMutations wraps an admin handler so that only MUTATING requests
	// (POST/PUT/PATCH/DELETE) are recorded to the org-scoped audit trail via
	// auditRec.Log — read-only GETs pass through unaudited so the log isn't
	// flooded with every console page-load. The wrapped handler still runs inside
	// driveAuthWrap at the call site, so the recorder resolves actor/org from the
	// auth context. Best-effort: a nil recorder yields the handler unchanged.
	auditMutations := func(service, action string, h http.Handler) http.Handler {
		audited := auditRec.Log(service, action, h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				audited.ServeHTTP(w, r)
			default:
				h.ServeHTTP(w, r)
			}
		})
	}

	// Refused (403) business calls are audited as permission_denied events.
	authz.SetDenyHook(func(ctx context.Context, d authz.Denial) {
		auditRec.RecordDenied(ctx, d.Permission, d.Method, d.Target, d.IP, d.UserAgent)
	})
	clientAuditEvent := auditRec.ClientEventHandler(map[string]string{
		"crm":        authz.CRMRead,
		"invoicing":  authz.InvoicingRead,
		"accounting": authz.AccountingRead,
	}, authz.Can)

	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Host-based dispatch: anything on the CRM subdomain goes straight to
		// Twenty at root. Must run before the path-based branches so every path
		// (/, /auth/oidc/callback, /graphql, assets, ws upgrades) on the CRM host
		// reaches Twenty. r.Host carries the authority incl. port.
		if crmProxy != nil && cfg.CRMHost != "" && hostMatches(r.Host, cfg.CRMHost) {
			crmProxy.ServeHTTP(w, r)
			return
		}
		// Identity: SCIM provisioning (bearer token auth), SSO discovery/start and the
		// notification emit hook (source token) are public by design: each does its own auth.
		if scimSrv != nil && scim.Match(r.URL.Path) {
			scimSrv.ServeHTTP(w, r)
			return
		}
		if ssoH != nil && sso.MatchPublic(r.URL.Path) {
			ssoH.ServePublic(w, r)
			return
		}
		if notifyHook != nil && strings.TrimRight(r.URL.Path, "/") == notifyhook.EmitPath {
			notifyHook.ServeEmit(w, r)
			return
		}
		// Public mail client config (MTA-STS policy, autoconfig, autodiscover,
		// mobileconfig): answered by Host/email for verified domains, else 404.
		if dnsMailH != nil && dnsmail.MatchPublic(r.URL.Path) {
			dnsMailH.ServeHTTP(w, r)
			return
		}
		// PDF backend under /pdf-api/. When the in-process builtin is on, adrient
		// serves it directly (auth + bridge); otherwise the legacy reverse-proxy
		// path is used. Exactly one claims the prefix.
		if pdfBuiltinHandler != nil && strings.HasPrefix(r.URL.Path, "/pdf-api/") {
			pdfBuiltinHandler.ServeHTTP(w, r)
			return
		}
		if pdfBackend != nil && strings.HasPrefix(r.URL.Path, "/pdf-api/") {
			pdfBackend.ServeHTTP(w, r)
			return
		}
		// PDF frontend under /pdf and /pdf/*. When the in-process static SPA is
		// configured (built-in + PDFStaticDir), adrient serves the SPA itself;
		// otherwise the legacy reverse-proxy to PDFFrontendURL is used. /pdf-api/*
		// is already claimed above, so it never reaches here.
		if pdfStaticHandler != nil && (r.URL.Path == "/pdf" || strings.HasPrefix(r.URL.Path, "/pdf/")) {
			pdfStaticHandler.ServeHTTP(w, r)
			return
		}
		if pdfFrontend != nil && (r.URL.Path == "/pdf" || strings.HasPrefix(r.URL.Path, "/pdf/")) {
			pdfFrontend.ServeHTTP(w, r)
			return
		}
		// Drive's binary endpoints (multipart upload, blob download) are NOT
		// gRPC-gateway routes; they're raw HTTP. Route them through the auth
		// middleware then directly to the drive handlers. Path is normalized
		// (trailing slash trimmed) before matching, since some clients/proxies
		// append a slash that an exact `==` check would miss.
		if cfg.Drive != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			// Share links: visibility is enforced inside the handler, so signed-out
			// visitors reach it (no app guard) and get a sign-in answer when needed.
			if strings.HasPrefix(path, "/api/v1/drive/link/") && r.Method == http.MethodGet {
				auditRec.Log("drive", "link-open", driveAuthBase(cfg.Drive.LinkHandler())).ServeHTTP(w, r)
				return
			}
			if path == "/api/v1/drive/files/upload" && r.Method == http.MethodPost {
				auditRec.Log("drive", "upload", driveAuthWrap(cfg.Drive.UploadHandler())).ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(path, "/api/v1/drive/files/") && strings.HasSuffix(path, "/content") && r.Method == http.MethodGet {
				// Version download: /api/v1/drive/files/{id}/versions/{vid}/content
				if strings.Contains(path, "/versions/") {
					auditRec.Log("drive", "version-download", driveAuthWrap(cfg.Drive.VersionDownloadHandler())).ServeHTTP(w, r)
					return
				}
				// Current-content download: /api/v1/drive/files/{id}/content
				auditRec.Log("drive", "download", driveAuthWrap(cfg.Drive.DownloadHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Mail saved searches + signatures (plain JSON, per user).
		if mailExtras != nil && mailExtras.Match(r.URL.Path) {
			auditRec.Log("mail", "extras", driveAuthWrap(mailExtras)).ServeHTTP(w, r)
			return
		}
		// Drive storage quota report.
		if cfg.Drive != nil && strings.TrimRight(r.URL.Path, "/") == "/api/v1/drive/quota" && r.Method == http.MethodGet {
			auditRec.Log("drive", "quota", driveAuthWrap(cfg.Drive.QuotaHandler())).ServeHTTP(w, r)
			return
		}
		// Mail attachments — raw multipart upload + blob download.
		if mailAtt != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			if h := mailAtt.Route(path, r.Method); h != nil {
				auditRec.Log("mail", "attachment-drive", driveAuthWrap(h)).ServeHTTP(w, r)
				return
			}
			if path == "/api/v1/mail/attachments" && r.Method == http.MethodPost {
				auditRec.Log("mail", "attachment-upload", driveAuthWrap(mailAtt.UploadHandler())).ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/mail/attachments/") && strings.HasSuffix(r.URL.Path, "/content") && r.Method == http.MethodGet {
				auditRec.Log("mail", "attachment-download", driveAuthWrap(mailAtt.DownloadHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Imported HTML games — multipart upload + list + sandboxed content.
		// Untrusted HTML: ContentHandler sets nosniff + frame-ancestors CSP and
		// the frontend loads it only in a sandboxed iframe (no allow-same-origin).
		if gamesH != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			if path == "/api/v1/games" && r.Method == http.MethodPost {
				auditRec.Log("games", "import", driveAuthWrap(gamesH.UploadHandler())).ServeHTTP(w, r)
				return
			}
			if path == "/api/v1/games" && r.Method == http.MethodGet {
				auditRec.Log("games", "list", driveAuthWrap(gamesH.ListHandler())).ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/games/") && strings.HasSuffix(r.URL.Path, "/content") && r.Method == http.MethodGet {
				auditRec.Log("games", "content", driveAuthWrap(gamesH.ContentHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Mail raw source — "Show original" header inspection.
		if mailBackend != nil {
			p := strings.TrimRight(r.URL.Path, "/")
			if strings.HasPrefix(p, "/api/v1/mail/messages/") && strings.HasSuffix(p, "/raw") && r.Method == http.MethodGet {
				auditRec.Log("mail", "raw-source", driveAuthWrap(mail.RawHandler(mailBackend))).ServeHTTP(w, r)
				return
			}
		}
		// Chat attachments — raw multipart upload + blob download (mirrors mail).
		if chatAtt != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			if path == "/api/v1/chat/attachments/upload" && r.Method == http.MethodPost {
				auditRec.Log("chat", "attachment-upload", driveAuthWrap(chatAtt.UploadHandler())).ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/chat/attachments/") && strings.HasSuffix(r.URL.Path, "/content") && r.Method == http.MethodGet {
				auditRec.Log("chat", "attachment-download", driveAuthWrap(chatAtt.DownloadHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Photos — raw image upload + download (the /content suffix distinguishes
		// the download from the gRPC-gateway GET /api/v1/photos/{id}).
		if photosMedia != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			if path == "/api/v1/photos/upload" && r.Method == http.MethodPost {
				auditRec.Log("photos", "upload", driveAuthWrap(photosMedia.UploadHandler())).ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/photos/") && strings.HasSuffix(r.URL.Path, "/content") && r.Method == http.MethodGet {
				auditRec.Log("photos", "download", driveAuthWrap(photosMedia.DownloadHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Books — raw book-file + cover upload/download.
		if booksFiles != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			if id, ok := books.FileID(path); ok && id != "" {
				switch r.Method {
				case http.MethodPost:
					auditRec.Log("books", "upload", driveAuthWrap(booksFiles.UploadFileHandler())).ServeHTTP(w, r)
					return
				case http.MethodGet:
					auditRec.Log("books", "download", driveAuthWrap(booksFiles.DownloadFileHandler())).ServeHTTP(w, r)
					return
				}
			}
			if id, ok := books.CoverID(path); ok && id != "" {
				switch r.Method {
				case http.MethodPost:
					auditRec.Log("books", "cover-upload", driveAuthWrap(booksFiles.UploadCoverHandler())).ServeHTTP(w, r)
					return
				case http.MethodGet:
					auditRec.Log("books", "cover-download", driveAuthWrap(booksFiles.CoverHandler())).ServeHTTP(w, r)
					return
				}
			}
		}
		// Music / Video app APIs (/api/v1/music/x/*, /api/v1/videos/x/*).
		if cfg.MusicExtras != nil && cfg.MusicExtras.Match(r.URL.Path) {
			driveAuthWrap(auditRec.LogAPI("music", "/api/v1/music/x", nil, cfg.MusicExtras)).ServeHTTP(w, r)
			return
		}
		if cfg.VideoExtras != nil && cfg.VideoExtras.Match(r.URL.Path) {
			driveAuthWrap(auditRec.LogAPI("video", "/api/v1/videos/x", nil, cfg.VideoExtras)).ServeHTTP(w, r)
			return
		}
		// Video — raw multipart upload + range-capable stream/download + captions.
		if videoHTTP != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			if path == "/api/v1/videos/upload" && r.Method == http.MethodPost {
				auditRec.Log("video", "upload", driveAuthWrap(videoHTTP.UploadHandler())).ServeHTTP(w, r)
				return
			}
			// Caption upload: must match before the generic /content route.
			if videoCaptions != nil && strings.HasSuffix(r.URL.Path, "/captions/upload") && r.Method == http.MethodPost {
				auditRec.Log("video", "caption_upload", driveAuthWrap(videoHTTP.CaptionUploadHandler(videoCaptions))).ServeHTTP(w, r)
				return
			}
			// Caption stream: /api/v1/videos/captions/{id}/content
			if videoCaptions != nil {
				if captionID, ok := video.CaptionID(r.URL.Path); ok && captionID != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
					auditRec.Log("video", "caption_stream", driveAuthWrap(videoHTTP.CaptionStreamHandler(videoCaptions))).ServeHTTP(w, r)
					return
				}
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/videos/") && strings.HasSuffix(r.URL.Path, "/content") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				auditRec.Log("video", "stream", driveAuthWrap(videoHTTP.StreamHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Music — raw multipart upload + range-capable stream/download.
		if musicHTTP != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			if path == "/api/v1/music/upload" && r.Method == http.MethodPost {
				auditRec.Log("music", "upload", driveAuthWrap(musicHTTP.UploadHandler())).ServeHTTP(w, r)
				return
			}
			// Radio: list / play / stop / retention / live stream proxy. Matched
			// before the generic /content stream so radio paths take precedence.
			if path == "/api/v1/music/radio/stations" && r.Method == http.MethodGet {
				auditRec.Log("music", "radio_stations", driveAuthWrap(musicHTTP.ListStationsHandler())).ServeHTTP(w, r)
				return
			}
			if path == "/api/v1/music/radio/stations" && r.Method == http.MethodPost {
				auditRec.Log("music", "radio_station_add", driveAuthWrap(musicHTTP.CreateStationHandler())).ServeHTTP(w, r)
				return
			}
			if _, action, ok := music.RadioStationID(r.URL.Path); ok {
				switch {
				case action == "play" && r.Method == http.MethodPost:
					auditRec.Log("music", "radio_play", driveAuthWrap(musicHTTP.PlayHandler())).ServeHTTP(w, r)
					return
				case action == "stop" && r.Method == http.MethodPost:
					auditRec.Log("music", "radio_stop", driveAuthWrap(musicHTTP.StopHandler())).ServeHTTP(w, r)
					return
				case action == "retention" && (r.Method == http.MethodGet || r.Method == http.MethodPut || r.Method == http.MethodPatch):
					auditRec.Log("music", "radio_retention", driveAuthWrap(musicHTTP.RetentionHandler())).ServeHTTP(w, r)
					return
				case action == "stream" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
					driveAuthWrap(musicHTTP.StreamProxyHandler()).ServeHTTP(w, r)
					return
				}
			}
			if strings.HasPrefix(r.URL.Path, "/api/v1/music/") && strings.HasSuffix(r.URL.Path, "/content") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				auditRec.Log("music", "stream", driveAuthWrap(musicHTTP.StreamHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Podcasts — per-user subscriptions + SSRF-guarded RSS feed proxy. All
		// auth-wrapped (authed users only), like the music routes.
		if podcastsHTTP != nil {
			path := strings.TrimRight(r.URL.Path, "/")
			switch {
			case path == "/api/v1/podcasts/subscriptions" && r.Method == http.MethodGet:
				auditRec.Log("podcasts", "list_subscriptions", driveAuthWrap(podcastsHTTP.ListSubscriptionsHandler())).ServeHTTP(w, r)
				return
			case path == "/api/v1/podcasts/subscriptions" && r.Method == http.MethodPost:
				auditRec.Log("podcasts", "subscribe", driveAuthWrap(podcastsHTTP.SubscribeHandler())).ServeHTTP(w, r)
				return
			case strings.HasPrefix(r.URL.Path, "/api/v1/podcasts/subscriptions/") && r.Method == http.MethodDelete:
				auditRec.Log("podcasts", "unsubscribe", driveAuthWrap(podcastsHTTP.UnsubscribeHandler())).ServeHTTP(w, r)
				return
			case path == "/api/v1/podcasts/feed" && r.Method == http.MethodGet:
				// SSRF-sensitive: fetches a user-supplied URL server-side. Audited.
				auditRec.Log("podcasts", "fetch_feed", driveAuthWrap(podcastsHTTP.FeedHandler())).ServeHTTP(w, r)
				return
			}
		}
		// Public video share-link routes — NOT auth-wrapped (token-validated).
		if videoHTTP != nil {
			if tok, ok := video.SharedContentToken(r.URL.Path); ok && tok != "" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				videoHTTP.SharedStreamHandler().ServeHTTP(w, r)
				return
			}
			if tok, ok := video.SharedTokenID(strings.TrimRight(r.URL.Path, "/")); ok && tok != "" && r.Method == http.MethodGet {
				videoHTTP.SharedMetaHandler().ServeHTTP(w, r)
				return
			}
		}
		// In-app account security: proxy /api/zitadel/* to the Zitadel User API
		// v2. Auth-wrapped so the session user is in context; the proxy enforces
		// userId == caller's oidc_subject.
		if strings.HasPrefix(r.URL.Path, zitadelproxy.MountPrefix+"/") {
			driveAuthWrap(zproxy).ServeHTTP(w, r)
			return
		}
		// Optional LibreOffice conversion (legacy .doc/.xls/.ppt → OOXML).
		if r.URL.Path == convert.CapabilitiesPath || r.URL.Path == convert.OfficePath {
			driveAuthWrap(officeConvertHandler).ServeHTTP(w, r)
			return
		}
		// Admin-gated audit-log JSON listing.
		if r.URL.Path == "/api/v1/admin/audit" {
			driveAuthWrap(auditHandler).ServeHTTP(w, r)
			return
		}
		// Admin-gated org usage analytics (read-only COUNT/SUM over all app tables).
		if r.URL.Path == "/api/v1/admin/analytics" {
			driveAuthWrap(analyticsHandler).ServeHTTP(w, r)
			return
		}
		// Admin-gated geo-location access policy (GET/PUT). Instance-level edge
		// access control enforced by geoMiddleware against CF-IPCountry. This
		// path lives under /api/v1/admin/ so geoMiddleware always exempts it —
		// an admin can never lock themselves out of the policy editor.
		if r.URL.Path == "/api/v1/admin/geo" {
			driveAuthWrap(auditMutations("security", "geo-access-change", geoHandler)).ServeHTTP(w, r)
			return
		}
		// Admin-gated honeypot console: recent intrusion alerts + counts (GET) and
		// clear/acknowledge (DELETE). Under /api/v1/admin/ so geoMiddleware exempts
		// it. The traps that FEED it (decoy paths + /api/v1/honeypot) are public.
		if r.URL.Path == "/api/v1/admin/honeypot" {
			driveAuthWrap(auditMutations("security", "honeypot-clear", honeypotAdmin)).ServeHTTP(w, r)
			return
		}
		// Admin-gated rate-limiting panel: effective config + recent 429 blocks +
		// top offending IPs (read-only). Under /api/v1/admin/ so geoMiddleware
		// exempts it. Instance-global data (the limiter keys on IP, not org).
		if r.URL.Path == "/api/v1/admin/ratelimit" {
			driveAuthWrap(rateLimitAdmin).ServeHTTP(w, r)
			return
		}
		// Admin-gated security console: reads/writes Zitadel org policies
		// (password / MFA / lockout / passwordless) scoped to the caller's org.
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/security/") {
			driveAuthWrap(auditMutations("security", "security-policy-change", securityHandler)).ServeHTTP(w, r)
			return
		}
		// User avatar: upload (POST), serve (GET), delete (DELETE) for the caller's
		// own avatar; GET for any user's avatar (org member).
		if avatarHandler != nil {
			p := strings.TrimRight(r.URL.Path, "/")
			if p == "/api/v1/me/avatar" ||
				(strings.HasPrefix(p, "/api/v1/users/") && strings.HasSuffix(p, "/avatar")) {
				driveAuthWrap(avatarHandler).ServeHTTP(w, r)
				return
			}
		}
		// Multi-account switching: list browser's accounts, activate one, remove one.
		if multiAccountHandler != nil {
			p := strings.TrimRight(r.URL.Path, "/")
			if p == "/api/v1/me/accounts" ||
				(strings.HasPrefix(p, "/api/v1/me/accounts/") && strings.HasSuffix(p, "/activate")) ||
				(strings.HasPrefix(p, "/api/v1/me/accounts/") && !strings.Contains(strings.TrimPrefix(p, "/api/v1/me/accounts/"), "/")) {
				driveAuthWrap(multiAccountHandler).ServeHTTP(w, r)
				return
			}
		}
		// Identity: verified domains + org sign-up settings (plan A), then user
		// lifecycle + invitations (plan B). Admin-gated inside the handlers; the
		// domain hint and invitation lookup are public and rate-limited.
		// Identity admin: SCIM tokens, SSO, seats and sign-in security, lifecycle,
		// access reviews, delegated roles and tokens, audit search, global search,
		// notification center and sources.
		if scimAdminH != nil && scim.MatchAdmin(r.URL.Path) {
			driveAuthWrap(scimAdminH).ServeHTTP(w, r)
			return
		}
		if ssoH != nil && sso.MatchAdmin(r.URL.Path) {
			driveAuthWrap(ssoH).ServeHTTP(w, r)
			return
		}
		if seatsH != nil && seats.Match(r.URL.Path) {
			driveAuthWrap(seatsH).ServeHTTP(w, r)
			return
		}
		if offH != nil && offboard.Match(r.URL.Path) {
			driveAuthWrap(offH).ServeHTTP(w, r)
			return
		}
		if reviewH != nil && accessreview.Match(r.URL.Path) {
			driveAuthWrap(reviewH).ServeHTTP(w, r)
			return
		}
		if iamH != nil && iamops.Match(r.URL.Path) {
			driveAuthWrap(iamH).ServeHTTP(w, r)
			return
		}
		if rolesH != nil && adminroles.Match(r.URL.Path) {
			driveAuthWrap(rolesH).ServeHTTP(w, r)
			return
		}
		if auditSearch != nil && r.URL.Path == auditsearch.Path {
			driveAuthWrap(auditSearch).ServeHTTP(w, r)
			return
		}
		if gsearch != nil && r.URL.Path == globalsearch.Path {
			driveAuthWrap(gsearch.Handler()).ServeHTTP(w, r)
			return
		}
		if notifyHook != nil && notifyhook.Match(r.URL.Path) && strings.TrimRight(r.URL.Path, "/") != notifyhook.EmitPath {
			driveAuthWrap(http.HandlerFunc(notifyHook.ServeSession)).ServeHTTP(w, r)
			return
		}
		if dnsMailH != nil && dnsmail.MatchAdmin(r.URL.Path) {
			driveAuthWrap(dnsMailH).ServeHTTP(w, r)
			return
		}
		if idDomainsH != nil && orgdomains.Match(r.URL.Path) {
			driveAuthWrap(idDomainsH).ServeHTTP(w, r)
			return
		}
		if idLifecycleH != nil && lifecycle.Match(r.URL.Path) {
			driveAuthWrap(idLifecycleH).ServeHTTP(w, r)
			return
		}
		// Org settings (rename), branding (logo + accent color), and sessions —
		// the decoupled, admin-gated orgadminhttp surface. The /admin/* paths are
		// admin-only; the /org/branding and /me/sessions paths are member routes
		// (the handler authorizes each branch itself). All auth-wrapped so the
		// caller's user/org/session-token are on the context. The branding logo
		// routes are audited (upload/download) like the other blob routes; the
		// rest are JSON config calls covered by the gRPC audit elsewhere.
		if p := strings.TrimRight(r.URL.Path, "/"); p == "/api/v1/admin/org" ||
			strings.HasPrefix(p, "/api/v1/admin/org/branding") ||
			p == "/api/v1/admin/sessions" || strings.HasPrefix(p, "/api/v1/admin/sessions/") ||
			p == "/api/v1/org/branding" || strings.HasPrefix(p, "/api/v1/org/branding/") ||
			p == "/api/v1/me/sessions" || strings.HasPrefix(p, "/api/v1/me/sessions/") {
			driveAuthWrap(auditMutations("admin", "org-settings-change", orgAdminHandler)).ServeHTTP(w, r)
			return
		}
		// Non-gated admin self-check: any authenticated member may ask whether
		// THEY are an admin (drives the dashboard "Add user" affordance). Must
		// precede the gated /users prefix.
		if r.URL.Path == "/api/v1/admin/whoami" {
			driveAuthWrap(http.HandlerFunc(adminUsers.WhoAmI)).ServeHTTP(w, r)
			return
		}
		// Admin user management (Zitadel-backed). Distinct path from the gRPC
		// AdminService's /api/v1/admin/service-settings.
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/users") {
			driveAuthWrap(auditMutations("admin", "user-management", adminUsers)).ServeHTTP(w, r)
			return
		}
		// Role assignments, app access policies and the caller's app list.
		if authzAdmin != nil && authzAdmin.Match(r.URL.Path) {
			driveAuthWrap(authzAdmin).ServeHTTP(w, r)
			return
		}
		// Per-app controls (rules, effective access viewer, /me/controls).
		if controlsH != nil && controlsH.Match(r.URL.Path) {
			driveAuthWrap(controlsH).ServeHTTP(w, r)
			return
		}
		// External collaboration policy, partner domains, share preview.
		if extPolicyH != nil && externalpolicy.Match(r.URL.Path) {
			driveAuthWrap(extPolicyH).ServeHTTP(w, r)
			return
		}
		// People directory / departments / org chart (audited mutations).
		if cfg.Pool != nil && people.Match(r.URL.Path) {
			driveAuthWrap(auditRec.LogAPI("directory", "/api/v1", nil, peopleHandler)).ServeHTTP(w, r)
			return
		}
		if cfg.Pool != nil && r.URL.Path == directory.SearchPath {
			driveAuthWrap(dirSearchHandler).ServeHTTP(w, r)
			return
		}
		// Org user directory search (member pickers).
		if r.URL.Path == "/api/v1/directory" {
			driveAuthWrap(dirHandler).ServeHTTP(w, r)
			return
		}
		// VPN status (Tailscale integration). Auth-wrapped.
		if cfg.VPNHandler != nil && r.URL.Path == "/api/v1/vpn/status" {
			driveAuthWrap(cfg.VPNHandler).ServeHTTP(w, r)
			return
		}
		// Published-apps registry (Access feature). GET = any member; mutations admin-gated.
		if accessHandler != nil && strings.HasPrefix(r.URL.Path, access.MountPrefix+"/") {
			driveAuthWrap(accessHandler).ServeHTTP(w, r)
			return
		}
		// On-demand container desktops (Guacamole Phase 2). Auth-wrapped.
		if desktopsHandler != nil && strings.HasPrefix(r.URL.Path, desktops.MountPrefix+"/") {
			driveAuthWrap(desktopsHandler).ServeHTTP(w, r)
			return
		}
		// Self-service profile editor: GET/PATCH /api/v1/me/profile (auth-wrapped).
		if r.URL.Path == "/api/v1/me/profile" {
			driveAuthWrap(profileHandler).ServeHTTP(w, r)
			return
		}
		// MediaMTX → adrient webhooks (server-to-server, NOT auth-wrapped). These
		// MUST precede the /api/ auth fallthrough. The auth webhook checks the
		// publish stream_key; the ready/notready hooks flip live/offline.
		if liveWebhooks != nil {
			if h, ok := liveWebhooks.Routes(r); ok {
				h.ServeHTTP(w, r)
				return
			}
		}
		// Forgejo → adrient webhook (server-to-server, HMAC-verified, NOT
		// session-auth-wrapped). MUST precede the /api/ auth fallthrough.
		if projectsSvc != nil && r.URL.Path == "/api/v1/forgejo/webhook" {
			projectsSvc.HandleForgejoWebhook(w, r)
			return
		}
		// MediaMTX playback proxies under adrient's origin. Auth-wrapped so only
		// signed-in org members reach MediaMTX (org-stream gating). Truly public
		// (signed-out) playback would require unwrapping these for public streams
		// — see the tradeoff note in internal/live/webhooks.go.
		if liveWebRTC != nil && strings.HasPrefix(r.URL.Path, "/live-webrtc/") {
			driveAuthWrap(liveWebRTC).ServeHTTP(w, r)
			return
		}
		if liveHLS != nil && strings.HasPrefix(r.URL.Path, "/live-hls/") {
			driveAuthWrap(liveHLS).ServeHTTP(w, r)
			return
		}
		// Meet SFU (LiveKit) join tokens + host controls — auth-wrapped, pure HTTP.
		if meetSFU != nil {
			// Anonymous (no account) join through a meeting link: public, rate
			// limited by the global limiter, always lands in the lobby.
			if _, ok := meet.MatchAnon(r.URL.Path); ok {
				meetSFU.ServeAnonymous(w, r)
				return
			}
			if _, _, ok := meetSFU.Match(r.URL.Path); ok {
				driveAuthWrap(meetSFU).ServeHTTP(w, r)
				return
			}
		}
		// Meet short-code surface (create + resolve) — auth-wrapped, pure HTTP.
		if meetCodes != nil {
			if _, ok := meetCodes.Match(r.URL.Path); ok {
				driveAuthWrap(meetCodes).ServeHTTP(w, r)
				return
			}
		}
		// Per-event meeting link surface (/calendar/events/{id}/meet) — must be
		// matched before the grpc-gateway, which has no route for this path.
		if eventMeetHTTP != nil {
			if _, ok := eventMeetHTTP.Match(r.URL.Path); ok {
				driveAuthWrap(eventMeetHTTP).ServeHTTP(w, r)
				return
			}
		}
		// Caller's effective RBAC permissions (UI hides what the caller cannot do).
		if r.URL.Path == "/api/v1/me/permissions" {
			driveAuthWrap(http.HandlerFunc(authz.MeHandler)).ServeHTTP(w, r)
			return
		}
		// Per-user API token management (/api/v1/me/tokens) — auth-wrapped, pure
		// HTTP, matched before the grpc-gateway.
		if apiTokensHTTP != nil {
			if _, ok := apiTokensHTTP.Match(r.URL.Path); ok {
				driveAuthWrap(apiTokensHTTP).ServeHTTP(w, r)
				return
			}
		}
		// Integration self-notify + identity (matched before the grpc-gateway,
		// which would otherwise read "push" as a notification id).
		if notifPushHTTP != nil && r.URL.Path == notifications.PushPath {
			driveAuthWrap(notifPushHTTP).ServeHTTP(w, r)
			return
		}
		if r.URL.Path == integrationinfo.HomeAssistantInfoPath {
			driveAuthWrap(haInfoHTTP).ServeHTTP(w, r)
			return
		}
		// Org Sync transfer (/api/v1/orgsync/transfer) — auth-wrapped.
		if orgSyncHTTP != nil && orgSyncHTTP.Match(r.URL.Path) {
			driveAuthWrap(orgSyncHTTP).ServeHTTP(w, r)
			return
		}
		// Client-side events worth auditing (CSV exports are built in the browser).
		if r.URL.Path == "/api/v1/audit/client-event" {
			driveAuthWrap(clientAuditEvent).ServeHTTP(w, r)
			return
		}
		// AI assistant (/api/v1/ai/*) — auth-wrapped; streams SSE.
		if aiHTTP != nil && aiHTTP.Match(r.URL.Path) {
			driveAuthWrap(authz.Guard(aiHTTP)).ServeHTTP(w, r)
			return
		}
		// CRM (/api/v1/crm/*) — auth-wrapped JSON API.
		if crmHTTP != nil && crmHTTP.Match(r.URL.Path) {
			driveAuthWrap(authz.Guard(auditRec.LogAPI("crm", "/api/v1/crm", nil, crmHTTP))).ServeHTTP(w, r)
			return
		}
		// Calendar booking pages (/api/v1/booking/*) — auth-wrapped JSON API.
		if bookingHTTP != nil && bookingHTTP.Match(r.URL.Path) {
			driveAuthWrap(bookingHTTP).ServeHTTP(w, r)
			return
		}
		// Help desk extras (/api/v1/ts/*) and Forms extras (/api/v1/fx/*) — auth-wrapped.
		if tsHTTP != nil && tsHTTP.Match(r.URL.Path) {
			driveAuthWrap(tsHTTP).ServeHTTP(w, r)
			return
		}
		if fxHTTP != nil && fxHTTP.Match(r.URL.Path) {
			driveAuthWrap(fxHTTP).ServeHTTP(w, r)
			return
		}
		// Public help centre and satisfaction survey — token only, no session.
		if tsPublic != nil && tsPublic.Match(r.URL.Path) {
			tsPublic.ServeHTTP(w, r)
			return
		}
		// Projects extras (/api/v1/px/*) — auth-wrapped JSON API.
		if pxHTTP != nil && pxHTTP.Match(r.URL.Path) {
			driveAuthWrap(pxHTTP).ServeHTTP(w, r)
			return
		}
		// Keep extras (/api/v1/kx/*) — auth-wrapped JSON/binary API.
		if kxHTTP != nil && kxHTTP.Match(r.URL.Path) {
			driveAuthWrap(kxHTTP).ServeHTTP(w, r)
			return
		}
		// Tasks extras (/api/v1/tx/*) — auth-wrapped JSON API.
		if txHTTP != nil && txHTTP.Match(r.URL.Path) {
			driveAuthWrap(txHTTP).ServeHTTP(w, r)
			return
		}
		// Contacts extras (/api/v1/cx/*) — auth-wrapped JSON API.
		if cxHTTP != nil && cxHTTP.Match(r.URL.Path) {
			driveAuthWrap(cxHTTP).ServeHTTP(w, r)
			return
		}
		// Calendar extras (/api/v1/calx/*) — auth-wrapped JSON API.
		if calxHTTP != nil && calxHTTP.Match(r.URL.Path) {
			driveAuthWrap(calxHTTP).ServeHTTP(w, r)
			return
		}
		// Tasks subscribe feed and shared read-only lists (/api/v1/public/tasks/*) — token only, no session.
		if txPublic != nil && txPublic.Match(r.URL.Path) {
			txPublic.ServeHTTP(w, r)
			return
		}
		// Private calendar feed (/api/v1/public/calendar/feed/{token}.ics) — token only, no session.
		if calxFeed != nil && calxFeed.Match(r.URL.Path) {
			calxFeed.ServeHTTP(w, r)
			return
		}
		// Public booking page for guests (/api/v1/public/booking/*) — no session.
		if bookingPublicHTTP != nil && bookingPublicHTTP.Match(r.URL.Path) {
			bookingPublicHTTP.ServeHTTP(w, r)
			return
		}
		// Stories (/api/v1/stories/*) — auth-wrapped JSON API plus audio streaming.
		if storiesHTTP != nil && storiesHTTP.Match(r.URL.Path) {
			driveAuthWrap(authz.Guard(auditRec.LogAPI("podcasts", podcasts.StoriesMount, nil, storiesHTTP))).ServeHTTP(w, r)
			return
		}
		// Talent (/api/v1/talent/*) — auth-wrapped JSON API.
		if talentHTTP != nil && talentHTTP.Match(r.URL.Path) {
			driveAuthWrap(talentHTTP).ServeHTTP(w, r)
			return
		}
		// Ops modules (/api/v1/shifts, /timetracking, /fieldservice, /fleet) — auth-wrapped JSON APIs.
		if oh, ok := opsHTTP.match(r.URL.Path); ok {
			driveAuthWrap(auditRec.LogAPI(oh.app, oh.mount, nil, oh.h)).ServeHTTP(w, r)
			return
		}
		// Knowledge modules (/api/v1/records, /wiki, ...) — auth-wrapped JSON APIs.
		for _, kh := range knowHTTP {
			if kh.Match(r.URL.Path) {
				driveAuthWrap(kh).ServeHTTP(w, r)
				return
			}
		}
		// HR (/api/v1/hr/*) — auth-wrapped JSON API.
		if hrHTTP != nil && hrHTTP.Match(r.URL.Path) {
			driveAuthWrap(auditRec.LogAPI("hr", "/api/v1/hr", nil, hrHTTP)).ServeHTTP(w, r)
			return
		}
		// Bridge (/api/v1/bridge/*) — cross-app hand-offs; each module re-checks permissions.
		if bridgeHTTP != nil && bridgeHTTP.Match(r.URL.Path) {
			driveAuthWrap(bridgeHTTP).ServeHTTP(w, r)
			return
		}
		// Invoicing (/api/v1/invoicing/*) — auth-wrapped JSON API.
		if invoicingHTTP != nil && invoicingHTTP.Match(r.URL.Path) {
			driveAuthWrap(authz.Guard(auditRec.LogAPI("invoicing", "/api/v1/invoicing", nil, invoicingHTTP))).ServeHTTP(w, r)
			return
		}
		// Inventory extensions (/api/v1/stock/*, /api/v1/purchasing/*) — auth-wrapped JSON API.
		if stockHTTP != nil && stockHTTP.Match(r.URL.Path) {
			driveAuthWrap(authz.Guard(auditRec.LogAPI("inventory", "/api/v1", nil, stockHTTP))).ServeHTTP(w, r)
			return
		}
		// POS wave-3 routes (/api/v1/pos/x/*) — auth-wrapped JSON API.
		if posExtHTTP != nil && posExtHTTP.Match(r.URL.Path) {
			driveAuthWrap(authz.Guard(auditRec.LogAPI("pos", "/api/v1/pos/x", nil, posExtHTTP))).ServeHTTP(w, r)
			return
		}
		// Accounting (/api/v1/accounting/*) — auth-wrapped JSON API.
		if accountingHTTP != nil && accountingHTTP.Match(r.URL.Path) {
			driveAuthWrap(authz.Guard(auditRec.LogAPI("accounting", "/api/v1/accounting", nil, accountingHTTP))).ServeHTTP(w, r)
			return
		}
		// Ticketing service (/api/v1/tickets/*) — auth-wrapped project/ticket API.
		if ticketsHTTP != nil && ticketsHTTP.Match(r.URL.Path) {
			driveAuthWrap(auditRec.LogAPI("tickets", "/api/v1/tickets", nil, ticketsHTTP)).ServeHTTP(w, r)
			return
		}
		// Telephony call-logging surface — auth-wrapped, pure HTTP.
		if telephonyCalls != nil {
			if telephonyCalls.Match(r.URL.Path) {
				driveAuthWrap(telephonyCalls).ServeHTTP(w, r)
				return
			}
		}
		// In-app password login — public, no auth wrapper needed.
		if r.URL.Path == "/api/v1/auth/login-password" {
			loginHandlers.PasswordLogin(w, r)
			return
		}
		// Demo login — capability probe (GET) + one-click sign-in (POST), public.
		if r.URL.Path == "/api/v1/auth/demo-login" {
			loginHandlers.DemoLogin(w, r)
			return
		}
		// Recently-updated games feed — public, read-only; drives the "NEW" badge
		// on the arcade. Stats the bundled games/*.html files (same StaticDir the
		// SPA + games are served from).
		if r.URL.Path == recentGamesPath {
			recentGamesHandler(cfg.StaticDir)(w, r)
			return
		}
		// Public 24h unique-visitor counter atop /games — no auth (same posture as
		// the recent-games feed). Returns {"unique_24h": N} from adrient.visits.
		if r.URL.Path == visits.ActiveUsersPath {
			visitsStore.Handler().ServeHTTP(w, r)
			return
		}
		// Cloud Import — multipart upload + job-status polling (auth-wrapped).
		if cloudImportHandler != nil && strings.HasPrefix(r.URL.Path, "/api/v1/import") {
			driveAuthWrap(cloudImportHandler).ServeHTTP(w, r)
			return
		}
		// Game-room ADMIN control plane — auth-wrapped + admin-gated (the handler
		// enforces the admin check). Checked before the public relay so the more
		// specific /admin/* paths win.
		if gameRoomsAdmin.Match(r.URL.Path) {
			driveAuthWrap(gameRoomsAdmin).ServeHTTP(w, r)
			return
		}
		// Public "recently updated games" feed — read-only, no auth (the bundled
		// games are public too). Stats <StaticDir>/games/*.html so the /games
		// frontend can show a NEW badge on freshly-updated games.
		if r.URL.Path == recentGamesPath {
			recentGamesHandler(cfg.StaticDir).ServeHTTP(w, r)
			return
		}
		// Public honeypot form trap (POST /api/v1/honeypot). PUBLIC by design — the
		// point is to catch unauthenticated bots that fill a hidden form field a
		// human never would. Bypasses the auth wall; always returns 204 so a bot
		// gets no signal. The admin LISTING (/api/v1/admin/honeypot) stays gated.
		if r.URL.Path == "/api/v1/honeypot" {
			honeypotStore.FormHandler().ServeHTTP(w, r)
			return
		}
		// Public game-room WS relay — joinable by link, no workspace account, so
		// it must bypass the auth wall (its access control is code + password).
		if gameRoomsHTTP.Match(r.URL.Path) {
			gameRoomsHTTP.ServeHTTP(w, r)
			return
		}
		// Bolo multiplayer (Orona authoritative WS server) — reverse-proxied at
		// /bolo-mp/*. Link-gated (a 20-letter gid in the URL), no adrient account,
		// so it bypasses the auth wall just like the game-room relay.
		if boloMpProxy != nil && strings.HasPrefix(r.URL.Path, "/bolo-mp/") {
			boloMpProxy.ServeHTTP(w, r)
			return
		}
		// Per-instance Forgejo (git hosting) at /git/* — reverse-proxied with the
		// /git prefix stripped. Forgejo does its own auth, so it bypasses the wall.
		if forgejoProxy != nil {
			if r.URL.Path == "/git" {
				http.Redirect(w, r, "/git/", http.StatusMovedPermanently)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/git/") {
				// Wrap with the auth middleware so the adrient session is resolved
				// (optionally) into context for reverse-proxy SSO; the director
				// reads it. Anonymous requests still pass through unauthenticated.
				forgejoAuthWrap(forgejoProxy).ServeHTTP(w, r)
				return
			}
		}
		// Assemble (spatial collaboration) at /assemble/* — reverse-proxied,
		// prefix stripped, BYPASSING adrient's auth wall so guests reach it without
		// a forced sign-in (Assemble handles SSO + guest access itself).
		if assembleProxy != nil {
			if r.URL.Path == "/assemble" {
				http.Redirect(w, r, "/assemble/", http.StatusMovedPermanently)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/assemble/") {
				assembleProxy.ServeHTTP(w, r)
				return
			}
		}
		// Learn platform at /learn/* — reverse-proxied to its own container, the
		// /learn prefix stripped, BYPASSING adrient's auth wall (Learn is public).
		// The PWA uses relative paths + a hash router, so it runs unchanged here.
		if learnProxy != nil {
			if r.URL.Path == "/learn" {
				http.Redirect(w, r, "/learn/", http.StatusMovedPermanently)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/learn/") {
				learnProxy.ServeHTTP(w, r)
				return
			}
		}
		// Alexa custom skill — both routes BYPASS adrient's auth wall on purpose:
		//   POST /alexa                                  — Alexa signs its own
		//     requests (the handler verifies the signature), so there is no adrient
		//     session; a forced sign-in would break the skill.
		//   GET  /api/v1/music/alexa/stream/{trackID}    — the Echo fetches audio
		//     with NO adrient session; the URL is HMAC-signed + time-limited instead.
		// Both must be matched BEFORE the /api/ auth-wall fallthrough below (the
		// stream path lives under /api/).
		if alexaHandler != nil {
			if r.URL.Path == "/alexa" {
				alexaHandler.Handler(w, r)
				return
			}
			if _, ok := alexa.StreamTrackID(r.URL.Path); ok && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				alexaHandler.StreamHandler().ServeHTTP(w, r)
				return
			}
		}
		// Public invoice share page data (/api/v1/public/invoices/{token}) — read-only,
		// unguessable token, no session.
		if invoicingPublicHTTP != nil && invoicingPublicHTTP.Match(r.URL.Path) {
			invoicingPublicHTTP.ServeHTTP(w, r)
			return
		}
		// Public signing links (/api/v1/public/signatures/<token>): the unguessable token is the credential.
		if cfg.Know != nil {
			for _, ph := range cfg.Know.PublicHandlers() {
				if ph.Match(r.URL.Path) {
					ph.ServeHTTP(w, r)
					return
				}
			}
		}
		// Public career page, application form and offer e-accept (no session).
		if talentPublicHTTP != nil && talentPublicHTTP.Match(r.URL.Path) {
			talentPublicHTTP.ServeHTTP(w, r)
			return
		}
		// Public CRM lead capture form (/api/v1/public/crm/lead/{token}): unguessable token, no session.
		if crmPublicHTTP != nil && crmPublicHTTP.Match(r.URL.Path) {
			crmPublicHTTP.ServeHTTP(w, r)
			return
		}
		// Public ticket intake (/api/v1/public/tickets/{token}) — unauthenticated
		// submission for projects that opted into a public intake link.
		if ticketsPublicHTTP != nil && ticketsPublicHTTP.Match(r.URL.Path) {
			ticketsPublicHTTP.ServeHTTP(w, r)
			return
		}
		// Home Assistant custom integration bundle (public; the chart's HA pod
		// and bring-your-own HA installs download it). See internal/hacomponent.
		if r.URL.Path == hacomponent.Path {
			haComponent.ServeHTTP(w, r)
			return
		}
		// Product version (public): About dialog, sign-in footer, public pages.
		if r.URL.Path == "/api/v1/version" {
			versionHTTP.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			authWrapped.ServeHTTP(w, r)
			return
		}
		// Public help site at /help (no auth). Its pages live as files under
		// /docs/*.html, which the static handler serves; bare /docs and /docs/
		// fall through to the SPA so the Docs app owns that path.
		if r.URL.Path == "/help" {
			http.Redirect(w, r, "/help/", http.StatusMovedPermanently)
			return
		}
		if r.URL.Path == "/help/" || r.URL.Path == "/docs/index.html" {
			serveStaticFile(w, r, cfg.StaticDir, "docs/index.html")
			return
		}
		static.ServeHTTP(w, r)
	})

	// Geo-location access control wraps the WHOLE router: every route (main app,
	// games area, all API + static) passes through geoMiddleware. When the policy
	// mode is off (the default) it is a no-op. When block/allow is active it
	// returns 403 for disallowed CF-IPCountry values — EXCEPT the always-exempt
	// recovery paths (/api/v1/admin/*, /api/v1/auth/*, health) so an admin can
	// never lock themselves out. Unknown/absent country headers fail open.
	// Visitor tracker wraps the router (inside geo/honeypot) so it observes real
	// navigations only — it records a hashed-IP daily visit for GET page/app
	// requests, skipping API/relay/static/bot noise (and decoy 404s, which the
	// honeypot layer short-circuits before this). Cheap, async, non-blocking.
	tracked := visitsStore.Middleware(router)
	geoMiddleware := geoCache.Middleware(tracked)

	// Honeypot decoy tripwire wraps the OUTERMOST layer so the fixed decoy paths
	// (/.env, /wp-login.php, …) are intercepted before anything else — recording
	// an alert and returning a plain 404 — while every real path falls straight
	// through to geoMiddleware/router. It only matches the exact decoy paths, so
	// it never shadows a real route, the geo middleware, or auth.
	honeypotMiddleware := honeypotStore.Middleware(geoMiddleware)

	return &Server{grpc: grpcSrv, httpHandler: honeypotMiddleware}
}

// orgAdminRoster adapts the org_admins + users repositories to the
// adminusers.AdminRoster interface, mapping Zitadel user ids (oidc_subject) to
// adrient user ids and exposing grant/revoke/count for the admin-role API. It
// reads the caller's identity off the auth context (so adminusers stays free of
// internal/auth's gen/ dependency).
type orgAdminRoster struct {
	orgAdmins *orgadmin.Repository
	users     *users.Repository
	issuer    string
}

func (r *orgAdminRoster) CallerUserID(ctx context.Context) (string, bool) {
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return "", false
	}
	return u.ID, true
}

func (r *orgAdminRoster) CallerOrgID(ctx context.Context) (string, bool) {
	org, ok := auth.OrgFromContext(ctx)
	if !ok {
		return "", false
	}
	return org.ID, true
}

// AdrientUserIDForZitadel resolves a Zitadel user id (oidc_subject) to a adrient
// user id within orgID. If the user has never signed into adrient, a minimal adrient
// row is provisioned (empty email/display_name, enriched on their next sign-in)
// so an admin can grant the role before the user's first login.
func (r *orgAdminRoster) AdrientUserIDForZitadel(ctx context.Context, orgID, zitadelID string) (string, error) {
	u, err := r.users.GetByOIDC(ctx, orgID, r.issuer, zitadelID)
	if err == nil {
		return u.ID, nil
	}
	if !errors.Is(err, users.ErrNotFound) {
		return "", err
	}
	provisioned, perr := r.users.UpsertByOIDC(ctx, users.UpsertInput{
		OrgID: orgID, OIDCIssuer: r.issuer, OIDCSubject: zitadelID,
	})
	if perr != nil {
		return "", perr
	}
	return provisioned.ID, nil
}

func (r *orgAdminRoster) AdminZitadelIDs(ctx context.Context, orgID string, zitadelIDs []string) (map[string]bool, error) {
	return r.orgAdmins.AdminUserIDsForZitadel(ctx, orgID, r.issuer, zitadelIDs)
}

func (r *orgAdminRoster) Grant(ctx context.Context, orgID, targetUserID, byUserID string) error {
	return r.orgAdmins.GrantAdmin(ctx, orgID, targetUserID, byUserID)
}

func (r *orgAdminRoster) Revoke(ctx context.Context, orgID, targetUserID string) error {
	return r.orgAdmins.RevokeAdmin(ctx, orgID, targetUserID)
}

// orgMembershipStore adapts the users repository to adminusers.OrgMembershipStore
// for the remove-from-org delete. It deletes the adrient.users row for a Zitadel id
// in the caller's org (org_admins cascades via its ON DELETE CASCADE FK). It
// NEVER touches Zitadel.
type orgMembershipStore struct {
	users  *users.Repository
	issuer string
}

func (s *orgMembershipStore) RemoveFromOrg(ctx context.Context, orgID, zitadelID string) (bool, error) {
	_, removed, err := s.users.DeleteByOIDC(ctx, orgID, s.issuer, zitadelID)
	return removed, err
}

func (r *orgAdminRoster) CountAdmins(ctx context.Context, orgID string) (int, error) {
	return r.orgAdmins.CountAdmins(ctx, orgID)
}

// ---- orgadminhttp store adapters -------------------------------------------
// These thin wrappers map adrient's concrete repos/blob store to the narrow
// interface types orgadminhttp declares, so that package stays decoupled from
// gen/ + internal/auth. A nil underlying repo yields a no-op/"unavailable" via
// the handler's own nil-store guards (the adapter value is non-nil but its
// field is nil, so we guard here too).

type orgStoreAdapter struct{ repo *orgs.Repository }

func (a orgStoreAdapter) UpdateDisplayName(ctx context.Context, id, displayName string) (orgadminhttp.Org, error) {
	o, err := a.repo.UpdateDisplayName(ctx, id, displayName)
	if err != nil {
		return orgadminhttp.Org{}, err
	}
	return orgadminhttp.Org{ID: o.ID, Slug: o.Slug, DisplayName: o.DisplayName}, nil
}

type brandingStoreAdapter struct{ repo *branding.Repository }

func (a brandingStoreAdapter) Get(ctx context.Context, orgID string) (orgadminhttp.Branding, error) {
	b, err := a.repo.Get(ctx, orgID)
	if err != nil {
		return orgadminhttp.Branding{}, err
	}
	return orgadminhttp.Branding{
		OrgID: b.OrgID, LogoBlobKey: b.LogoBlobKey, LogoMIME: b.LogoMIME, AccentColor: b.AccentColor, ProductName: b.ProductName,
	}, nil
}

func (a brandingStoreAdapter) SetAccentColor(ctx context.Context, orgID, accent string) error {
	return a.repo.SetAccentColor(ctx, orgID, accent)
}

func (a brandingStoreAdapter) SetProductName(ctx context.Context, orgID, name string) error {
	return a.repo.SetProductName(ctx, orgID, name)
}

func (a brandingStoreAdapter) SetLogo(ctx context.Context, orgID, blobKey, mime string) error {
	return a.repo.SetLogo(ctx, orgID, blobKey, mime)
}

type brandingBlobAdapter struct{ blobs *drive.Blobs }

func (a brandingBlobAdapter) Put(ctx context.Context, key, mimeType string, size int64, body io.Reader) error {
	return a.blobs.Put(ctx, key, mimeType, size, body)
}

func (a brandingBlobAdapter) Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	return a.blobs.Get(ctx, key)
}

type sessionStoreAdapter struct{ store *auth.SessionStore }

func (a sessionStoreAdapter) ListByOrg(ctx context.Context, orgID, currentToken string) ([]orgadminhttp.SessionInfo, error) {
	infos, err := a.store.ListByOrg(ctx, orgID, currentToken)
	if err != nil {
		return nil, err
	}
	return toHTTPSessions(infos), nil
}

func (a sessionStoreAdapter) ListByOrgPage(ctx context.Context, orgID, currentToken, state, q string, limit, offset int) ([]orgadminhttp.SessionInfo, error) {
	infos, err := a.store.ListByOrgPage(ctx, orgID, currentToken, state, q, limit, offset)
	if err != nil {
		return nil, err
	}
	return toHTTPSessions(infos), nil
}

func (a sessionStoreAdapter) ListByUser(ctx context.Context, userID, currentToken string) ([]orgadminhttp.SessionInfo, error) {
	infos, err := a.store.ListByUser(ctx, userID, currentToken)
	if err != nil {
		return nil, err
	}
	return toHTTPSessions(infos), nil
}

func (a sessionStoreAdapter) RevokeByOrgAndID(ctx context.Context, orgID, id string) (bool, error) {
	return a.store.RevokeByOrgAndID(ctx, orgID, id)
}

func (a sessionStoreAdapter) RevokeByUserAndID(ctx context.Context, userID, id string) (bool, error) {
	return a.store.RevokeByUserAndID(ctx, userID, id)
}

// toHTTPSessions maps auth.SessionInfo to orgadminhttp.SessionInfo.
func toHTTPSessions(in []auth.SessionInfo) []orgadminhttp.SessionInfo {
	out := make([]orgadminhttp.SessionInfo, 0, len(in))
	for _, s := range in {
		out = append(out, orgadminhttp.SessionInfo{
			ID: s.ID, UserID: s.UserID, Email: s.Email, DisplayName: s.DisplayName,
			CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt, LastSeenAt: s.LastSeenAt,
			RevokedAt: s.RevokedAt, IP: s.IP, UserAgent: s.UserAgent, Current: s.Current,
		})
	}
	return out
}

// ---- useravatar blob adapter ------------------------------------------------

type avatarBlobAdapter struct{ blobs *drive.Blobs }

func (a avatarBlobAdapter) Put(ctx context.Context, key, mimeType string, size int64, body io.Reader) error {
	return a.blobs.Put(ctx, key, mimeType, size, body)
}

func (a avatarBlobAdapter) Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	return a.blobs.Get(ctx, key)
}

func (a avatarBlobAdapter) Delete(ctx context.Context, key string) error {
	return a.blobs.Delete(ctx, key)
}

// ---- multiaccounts session lookup adapter ----------------------------------

// sessionLookupAdapter adapts auth.SessionStore + users/orgs repos to the
// multiaccounts.SessionLookup interface, keeping internal/multiaccounts free
// of internal/auth's gen/ dependency.
type sessionLookupAdapter struct {
	sessions *auth.SessionStore
	users    *users.Repository
	orgs     *orgs.Repository
}

func (a *sessionLookupAdapter) LookupFull(ctx context.Context, token string) (userID, email, displayName, orgID, orgName, orgSlug, publicID string, ok bool) {
	sess, err := a.sessions.Lookup(ctx, token)
	if err != nil {
		return
	}
	u, uerr := a.users.GetByID(ctx, sess.UserID)
	if uerr != nil {
		return
	}
	var oName, oSlug string
	if a.orgs != nil {
		o, oerr := a.orgs.GetByID(ctx, u.OrgID)
		if oerr == nil {
			oName = o.DisplayName
			oSlug = o.Slug
		}
	}
	pubID, _ := a.sessions.LookupPublicID(ctx, token)
	return u.ID, u.Email, u.DisplayName, u.OrgID, oName, oSlug, pubID, true
}

func (a *sessionLookupAdapter) LookupTokenByPublicID(ctx context.Context, publicID string) (string, error) {
	return a.sessions.LookupTokenByPublicID(ctx, publicID)
}

// hostMatches reports whether the request Host equals the configured CRM host,
// comparing case-insensitively and tolerating a port mismatch (a client may or
// may not include adrient's :8080). So "crm.workspace.localtest.me" and
// "crm.workspace.localtest.me:8080" both match either configured form.
func hostMatches(reqHost, want string) bool {
	strip := func(h string) string {
		h = strings.ToLower(strings.TrimSpace(h))
		if i := strings.LastIndex(h, ":"); i >= 0 {
			h = h[:i]
		}
		return h
	}
	return strip(reqHost) == strip(want)
}

// newReverseProxy builds a reverse proxy to target (e.g. the PDF frontend),
// preserving the request path. Returns nil when target is empty.
func newReverseProxy(target string) *httputil.ReverseProxy {
	u, err := url.Parse(target)
	if err != nil || target == "" {
		return nil
	}
	p := httputil.NewSingleHostReverseProxy(u)
	// Preserve the upstream host so its dev server / router matches correctly.
	orig := p.Director
	p.Director = func(r *http.Request) { orig(r); r.Host = u.Host }
	return p
}

// newStripPrefixProxy builds a reverse proxy to target that strips prefix from
// the request path before forwarding (e.g. /pdf-api/api/x → backend /api/x).
// Returns nil when target is empty.
func newStripPrefixProxy(target, prefix string) *httputil.ReverseProxy {
	u, err := url.Parse(target)
	if err != nil || target == "" {
		return nil
	}
	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
			p := strings.TrimPrefix(r.URL.Path, prefix)
			if p == "" {
				p = "/"
			}
			r.URL.Path = p
		},
	}
}

// newForgejoProxy builds the /git reverse proxy with adrient→Forgejo SSO via
// Forgejo's reverse-proxy authentication (X-WEBAUTH-USER / X-WEBAUTH-EMAIL) plus
// access-time org/team provisioning.
//
// Security model: the request has ALREADY passed through adrient's auth middleware
// (so the adrient user, if any, is in r.Context()). The director:
//
//  1. UNCONDITIONALLY strips any client-supplied X-WEBAUTH-* headers FIRST, so a
//     visitor can never forge an identity by sending those headers themselves.
//  2. Sets X-WEBAUTH-USER / X-WEBAUTH-EMAIL server-side ONLY for an authenticated
//     adrient session. Forgejo (ENABLE_REVERSE_PROXY_AUTH + AUTO_REGISTER) then
//     auto-creates + auto-logs-in the matching user with zero extra clicks.
//  3. Anonymous visitors get NO header → Forgejo serves them as a guest (public
//     repo browsing still works).
//
// When an authenticated user is present, it also kicks off best-effort, cached,
// non-blocking org/team provisioning in a goroutine (provisioner is a no-op when
// unconfigured, e.g. on prod adrient.com).
func newForgejoProxy(target, prefix string, prov *forgejo.Provisioner, isAdmin func(context.Context) bool) *httputil.ReverseProxy {
	u, err := url.Parse(target)
	if err != nil || target == "" {
		return nil
	}
	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
			p := strings.TrimPrefix(r.URL.Path, prefix)
			if p == "" {
				p = "/"
			}
			r.URL.Path = p

			// (1) Always drop any inbound reverse-proxy auth headers BEFORE we
			// decide whether to set them — prevents client impersonation.
			r.Header.Del("X-WEBAUTH-USER")
			r.Header.Del("X-WEBAUTH-EMAIL")

			ctx := r.Context()
			user, hasUser := auth.UserFromContext(ctx)
			if !hasUser || user.Email == "" {
				return // anonymous: no SSO header, guest browsing
			}

			// (2) Trusted, server-side identity for an authenticated session.
			r.Header.Set("X-WEBAUTH-USER", forgejo.UsernameFromEmail(user.Email))
			r.Header.Set("X-WEBAUTH-EMAIL", user.Email)

			// (3) Access-time org/team provisioning (best-effort, non-blocking).
			if prov == nil || !prov.Configured() {
				return
			}
			org, hasOrg := auth.OrgFromContext(ctx)
			if !hasOrg || org.Slug == "" {
				return
			}
			admin := isAdmin != nil && isAdmin(ctx)
			slug, display, email := org.Slug, org.DisplayName, user.Email
			go func() {
				// Detached context: outlive the proxied request, but bound it.
				bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				prov.EnsureAccess(bg, slug, display, email, admin)
			}()
		},
	}
}

// HTTPHandler returns the HTTP/REST handler (driven by grpc-gateway + middleware).
func (s *Server) HTTPHandler() http.Handler { return s.httpHandler }

// GRPC returns the underlying *grpc.Server.
func (s *Server) GRPC() *grpc.Server { return s.grpc }

// docsConnectID returns the document id from a collab WebSocket path of the form
// /api/v1/docs/d/{id}/connect, and whether the path matched.
func docsConnectID(path string) (string, bool) {
	const prefix = "/api/v1/docs/d/"
	const suffix = "/connect"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// serveDocsWS authorizes the collab WebSocket request and hands it to the hub.
// Three paths grant access: (1) an authenticated org member whose org owns the
// document (full edit), (2) an authenticated per-user grantee (object_grants),
// whose role determines read/write (cross-org), or (3) a valid share-link token
// for the document. Viewers/commenters connect read-only.
func serveDocsWS(w http.ResponseWriter, r *http.Request, id string, repo *docs.Repository, grants *sharing.Repository, hub *docs.Hub) {
	acc := docsAccessFor(r, id, repo, docsGrantLookup(grants))
	if !acc.Read {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	canWrite, isOwner := acc.Write, acc.Owner
	// Read-only document protection (Docs M10): only the owner writes.
	gate := &docs.ProtectionGate{Load: func() (string, error) { return repo.GetProtection(context.Background(), id) }}
	hub.ServeFunc(w, r, id, docsWriteGate(canWrite, isOwner, gate))
}

// sheetsConnectID returns the sheet id from /api/v1/sheets/d/{id}/connect.
func sheetsConnectID(path string) (string, bool) {
	const prefix = "/api/v1/sheets/d/"
	const suffix = "/connect"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// serveSheetsWS authorizes the sheet collab WebSocket and hands it to the
// broadcast hub. Two paths grant access: (1) an org member whose org owns the
// sheet (full edit), or (2) a per-user grantee (object_grants), whose role
// determines read/write (cross-org). Viewers/commenters connect read-only.
func serveSheetsWS(w http.ResponseWriter, r *http.Request, id string, repo *sheets.Repository, grants *sharing.Repository, hub *sheets.Hub) {
	ctx := r.Context()
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	authorized, canWrite := false, false
	if org, ok := auth.OrgFromContext(ctx); ok {
		if _, err := repo.Get(ctx, org.ID, id); err == nil {
			authorized, canWrite = true, true
		}
	}
	// Per-user grant path: a non-org-member with a grant may connect; only an
	// editor grant gets write.
	if !authorized && grants != nil {
		if role, ok, err := grants.RoleFor(ctx, u.ID, sharing.TypeSheetsSheet, id); err == nil && ok {
			if _, derr := repo.GetByID(ctx, id); derr == nil {
				authorized = true
				canWrite = sharing.CanWrite(role)
			}
		}
	}
	if !authorized {
		http.Error(w, "sheet not found", http.StatusNotFound)
		return
	}
	// Ops on protected cells are dropped for users the protections don't list.
	guard := &sheets.OpGuard{
		Load: func() (string, error) {
			sh, err := repo.GetByID(context.Background(), id)
			return sh.Data, err
		},
	}
	if sh, err := repo.GetByID(ctx, id); err == nil {
		guard.Editor = sheets.Editor{User: u.ID, Owner: sh.OwnerID}
	} else {
		guard.Editor = sheets.Editor{User: u.ID}
	}
	hub.ServeGuarded(w, r, id, canWrite, guard)
}

// slidesConnectID returns the deck id from /api/v1/slides/d/{id}/connect.
func slidesConnectID(path string) (string, bool) {
	const prefix = "/api/v1/slides/d/"
	const suffix = "/connect"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// serveSlidesWS authorizes the deck collab WebSocket and hands it to the
// broadcast hub. Two paths grant access: (1) an org member whose org owns the
// deck (full edit), or (2) a per-user grantee (object_grants), whose role
// determines read/write (cross-org). Viewers/commenters connect read-only.
func serveSlidesWS(w http.ResponseWriter, r *http.Request, id string, repo *slides.Repository, grants *sharing.Repository, hub *slides.Hub) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	authorized, canWrite := slidesDeckAccess(r, id, repo, grants)
	if !authorized {
		http.Error(w, "deck not found", http.StatusNotFound)
		return
	}
	hub.Serve(w, r, id, canWrite)
}

// slidesDeckAccess reports read/write access to a deck: an org member whose
// org owns it (read+write), or a per-user grantee (write only with an
// editor grant). Shared by the collab WebSocket and the deck assets.
func slidesDeckAccess(r *http.Request, id string, repo *slides.Repository, grants *sharing.Repository) (read, write bool) {
	ctx := r.Context()
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		return false, false
	}
	if org, ok := auth.OrgFromContext(ctx); ok {
		if _, err := repo.Get(ctx, org.ID, id); err == nil {
			return true, true
		}
	}
	if grants != nil {
		if role, ok, err := grants.RoleFor(ctx, u.ID, sharing.TypeSlidesDeck, id); err == nil && ok {
			if _, derr := repo.GetByID(ctx, id); derr == nil {
				return true, sharing.CanWrite(role)
			}
		}
	}
	return false, false
}

// newDocsAssets wires the document image assets to the same access check as
// the collab WebSocket (org member, grantee or share-link token).
func newDocsAssets(blobs docs.AssetBlobStore, repo *docs.Repository, grants *sharing.Repository) *docs.Assets {
	return docs.NewAssets(blobs, func(r *http.Request, id string) (bool, bool) {
		acc := docsAccessFor(r, id, repo, docsGrantLookup(grants))
		return acc.Read, acc.Write
	})
}

// newSlidesAssets wires the deck assets to the deck access check.
func newSlidesAssets(blobs slides.AssetBlobStore, repo *slides.Repository, grants *sharing.Repository) *slides.Assets {
	return slides.NewAssets(blobs, func(r *http.Request, id string) (bool, bool) {
		return slidesDeckAccess(r, id, repo, grants)
	})
}

// whiteboardsConnectID returns the board id from /api/v1/whiteboards/d/{id}/connect.
func whiteboardsConnectID(path string) (string, bool) {
	const prefix = "/api/v1/whiteboards/d/"
	const suffix = "/connect"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// serveWhiteboardsWS authorizes the board collab WebSocket and hands it to the
// broadcast hub. Two paths grant access: (1) an org member whose org owns the
// board (full edit), or (2) a per-user grantee (object_grants), whose role
// determines read/write (cross-org). Viewers/commenters connect read-only.
func serveWhiteboardsWS(w http.ResponseWriter, r *http.Request, id string, repo *whiteboards.Repository, grants *sharing.Repository, hub *whiteboards.Hub) {
	ctx := r.Context()
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	authorized, canWrite := false, false
	if org, ok := auth.OrgFromContext(ctx); ok {
		if _, err := repo.Get(ctx, org.ID, id); err == nil {
			authorized, canWrite = true, true
		}
	}
	// Per-user grant path: a non-org-member with a grant may connect; only an
	// editor grant writes.
	if !authorized && grants != nil {
		if role, ok, err := grants.RoleFor(ctx, u.ID, sharing.TypeWhiteboardBoard, id); err == nil && ok {
			if _, derr := repo.GetByID(ctx, id); derr == nil {
				authorized = true
				canWrite = sharing.CanWrite(role)
			}
		}
	}
	if !authorized {
		http.Error(w, "whiteboard not found", http.StatusNotFound)
		return
	}
	hub.Serve(w, r, id, canWrite)
}

// serveDocsConvert converts client-rendered HTML to a downloadable format via
// pandoc. The document content lives client-side as a Yjs CRDT, so the client
// posts its rendered HTML here; ?to= selects the format and ?name= the filename.
// Stored pictures (asset URLs) are read from the docs asset store for the
// documents the caller can read (assets may be nil: no blob store).
func serveDocsConvert(w http.ResponseWriter, r *http.Request, assets *docs.Assets) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	to := r.URL.Query().Get("to")
	if _, ok := docs.ConvertSupported(to); !ok {
		http.Error(w, "unsupported format", http.StatusBadRequest)
		return
	}
	html, ok := readConvertBody(w, r)
	if !ok {
		return
	}
	data, f, err := docs.ConvertHTMLWith(r.Context(), html, to, docs.ExportOptions{Assets: assets.Loader(r)})
	switch {
	case errors.Is(err, docs.ErrPDFUnavailable):
		// A missing engine is a server configuration, not a failure: 501 so
		// clients (and monitoring) can tell it apart from a broken conversion.
		http.Error(w, err.Error(), http.StatusNotImplemented)
		return
	case errors.Is(err, docs.ErrPandocUnavailable):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	case err != nil:
		http.Error(w, "conversion failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "document"
	}
	w.Header().Set("Content-Type", f.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+"."+f.Ext))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveDocsConvertCapabilities reports which ?to= formats serveDocsConvert
// can produce on this server (pandoc and a PDF engine are optional binaries).
func serveDocsConvertCapabilities(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(docs.Capabilities())
}

// serveDocsImport is the inverse of serveDocsConvert: it converts an uploaded
// document (docx/odt/rtf/epub/md/html/txt) to HTML via pandoc and returns that
// HTML. ?from= selects the source format. The document body lives client-side as
// a Yjs CRDT, so — mirroring convert — this endpoint is a stateless transform:
// the client seeds a freshly created doc (CreateDoc) with the returned HTML
// rather than the server writing into the CRDT.
func serveDocsImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	from := r.URL.Query().Get("from")
	if _, ok := docs.ImportSupported(from); !ok {
		http.Error(w, "unsupported format", http.StatusBadRequest)
		return
	}
	data, ok := readConvertBody(w, r)
	if !ok {
		return
	}
	html, err := docs.ImportToHTML(r.Context(), data, from)
	if err != nil {
		// pandoc refusing the file is the uploader's problem (422); anything
		// else is ours. Neither echoes pandoc's output (temp paths, stderr).
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			http.Error(w, "import failed: the file could not be read as "+from, http.StatusUnprocessableEntity)
			return
		}
		slog.Error("docs import", "from", from, "err", err)
		http.Error(w, "import failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

// readConvertBody reads a conversion input, refusing (413) one over the
// converter's limit rather than silently converting a truncated file.
func readConvertBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	data, err := io.ReadAll(io.LimitReader(r.Body, docs.MaxConvertBytes+1))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return nil, false
	}
	if len(data) > docs.MaxConvertBytes {
		http.Error(w, fmt.Sprintf("file too large (max %d bytes)", docs.MaxConvertBytes), http.StatusRequestEntityTooLarge)
		return nil, false
	}
	return data, true
}

// redirectOnAuthURL converts AuthService.Login responses into HTTP 302 redirects
// to the IdP authorization URL. Also converts Callback responses into 302
// redirects to the post-login URL.
func redirectOnAuthURL(_ context.Context, w http.ResponseWriter, msg proto.Message) error {
	switch m := msg.(type) {
	case *adrientv1.LoginResponse:
		if u := m.GetAuthorizationUrl(); u != "" {
			w.Header().Set("Location", u)
			w.WriteHeader(http.StatusFound)
			return nil
		}
	case *adrientv1.CallbackResponse:
		if u := m.GetRedirectTo(); u != "" {
			w.Header().Set("Location", u)
			w.WriteHeader(http.StatusFound)
			return nil
		}
	}
	return nil
}

// chatConnectID extracts the channel id from /api/v1/chat/channels/{id}/connect.
func chatConnectID(path string) (string, bool) {
	const prefix = "/api/v1/chat/channels/"
	const suffix = "/connect"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// serveChatWS authorizes the chat WebSocket (org member of the channel) and
// hands it to the broadcast/presence hub.
func serveChatWS(w http.ResponseWriter, r *http.Request, channelID string, repo *chat.Repository, hub *chat.Hub) {
	ctx := r.Context()
	u, hasUser := auth.UserFromContext(ctx)
	if !hasUser {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	org, ok := auth.OrgFromContext(ctx)
	if !ok {
		http.Error(w, "no org context", http.StatusInternalServerError)
		return
	}
	ch, err := repo.GetChannel(ctx, org.ID, channelID)
	if err != nil {
		http.Error(w, "channel not found", http.StatusNotFound)
		return
	}
	isMember := false
	for _, id := range ch.MemberIDs {
		if id == u.ID {
			isMember = true
			break
		}
	}
	if !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	hub.Serve(w, r, channelID, u.ID)
}

// serveMeetWS authorizes the Meet signaling WebSocket (org member whose org owns
// the room) and hands it to the signaling hub.
func serveMeetWS(w http.ResponseWriter, r *http.Request, roomID string, repo *meet.Repository, hub *meet.Hub) {
	ctx := r.Context()
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	org, ok := auth.OrgFromContext(ctx)
	if !ok {
		http.Error(w, "no org context", http.StatusInternalServerError)
		return
	}
	room, err := repo.Get(ctx, org.ID, roomID)
	if err != nil {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}
	if room.Ended() {
		http.Error(w, "meeting_ended", http.StatusGone)
		return
	}
	displayName := u.DisplayName
	if displayName == "" {
		displayName = u.Email
	}
	if displayName == "" {
		displayName = u.ID
	}
	// The green room lets a member choose the name shown in the call, and a
	// per-tab connection id lets one user join from several tabs/devices.
	displayName = meet.CleanDisplayName(r.URL.Query().Get("name"), displayName)
	hub.Serve(w, r, roomID, meet.PeerID(u.ID, r.URL.Query().Get("cid")), displayName)
}

// serveTelephonyWS authorizes the telephony signaling WebSocket (any org member)
// and hands it to the signaling hub, keyed by the caller's org + user id.
func serveTelephonyWS(w http.ResponseWriter, r *http.Request, hub *telephony.Hub) {
	ctx := r.Context()
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	org, ok := auth.OrgFromContext(ctx)
	if !ok {
		http.Error(w, "no org context", http.StatusInternalServerError)
		return
	}
	displayName := u.DisplayName
	if displayName == "" {
		displayName = u.Email
	}
	if displayName == "" {
		displayName = u.ID
	}
	hub.Serve(w, r, org.ID, u.ID, displayName)
}

// projectsConnectID extracts the team id from /api/v1/projects/teams/{id}/connect.
func projectsConnectID(path string) (string, bool) {
	const prefix = "/api/v1/projects/teams/"
	const suffix = "/connect"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// serveProjectsWS authorizes the projects WebSocket (org owns the team) and
// hands it to the issue broadcast/presence hub.
func serveProjectsWS(w http.ResponseWriter, r *http.Request, teamID string, repo *projects.Repository, hub *projects.Hub) {
	ctx := r.Context()
	u, ok := auth.UserFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	org, ok := auth.OrgFromContext(ctx)
	if !ok {
		http.Error(w, "no org context", http.StatusInternalServerError)
		return
	}
	if _, err := repo.GetTeam(ctx, org.ID, teamID); err != nil {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	hub.Serve(w, r, teamID, u.ID)
}
