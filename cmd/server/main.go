// Command server runs the adrient-workspace backend: gRPC on 9000, HTTP/REST on 8080.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/access"
	"github.com/Adrient-tech/Adrient-Workspace/internal/admin"
	"github.com/Adrient-tech/Adrient-Workspace/internal/audit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgdomains"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/books"
	"github.com/Adrient-tech/Adrient-Workspace/internal/branding"
	"github.com/Adrient-tech/Adrient-Workspace/internal/booking"
	"github.com/Adrient-tech/Adrient-Workspace/internal/buildinfo"
	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/chat"
	"github.com/Adrient-tech/Adrient-Workspace/internal/cloudimport"
	"github.com/Adrient-tech/Adrient-Workspace/internal/contacts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/convert"
	"github.com/Adrient-tech/Adrient-Workspace/internal/crm"
	"github.com/Adrient-tech/Adrient-Workspace/internal/eventmeet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/know"
	"github.com/Adrient-tech/Adrient-Workspace/internal/talent"
	"github.com/Adrient-tech/Adrient-Workspace/internal/hr"
	"github.com/Adrient-tech/Adrient-Workspace/internal/desktops"
	"github.com/Adrient-tech/Adrient-Workspace/internal/docs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
	"github.com/Adrient-tech/Adrient-Workspace/internal/forgejo"
	"github.com/Adrient-tech/Adrient-Workspace/internal/forms"
	"github.com/Adrient-tech/Adrient-Workspace/internal/games"
	"github.com/Adrient-tech/Adrient-Workspace/internal/groups"
	"github.com/Adrient-tech/Adrient-Workspace/internal/inventory"
	"github.com/Adrient-tech/Adrient-Workspace/internal/accounting"
	"github.com/Adrient-tech/Adrient-Workspace/internal/ai"
	"github.com/Adrient-tech/Adrient-Workspace/internal/invoicing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/keep"
	"github.com/Adrient-tech/Adrient-Workspace/internal/live"
	"github.com/Adrient-tech/Adrient-Workspace/internal/mail"
	"github.com/Adrient-tech/Adrient-Workspace/internal/meet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/multiaccounts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/music"
	"github.com/Adrient-tech/Adrient-Workspace/internal/music/radio"
	"github.com/Adrient-tech/Adrient-Workspace/internal/notifications"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgadmin"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgs"
	pdfapp "github.com/Adrient-tech/Adrient-Workspace/internal/pdf/app"
	"github.com/Adrient-tech/Adrient-Workspace/internal/photos"
	"github.com/Adrient-tech/Adrient-Workspace/internal/podcasts"
	"github.com/Adrient-tech/Adrient-Workspace/internal/pos"
	"github.com/Adrient-tech/Adrient-Workspace/internal/prefs"
	"github.com/Adrient-tech/Adrient-Workspace/internal/projects"
	"github.com/Adrient-tech/Adrient-Workspace/internal/search"
	"github.com/Adrient-tech/Adrient-Workspace/internal/server"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sharing"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sheets"
	"github.com/Adrient-tech/Adrient-Workspace/internal/sites"
	"github.com/Adrient-tech/Adrient-Workspace/internal/slides"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tasks"
	"github.com/Adrient-tech/Adrient-Workspace/internal/telephony"
	"github.com/Adrient-tech/Adrient-Workspace/internal/useravatar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
	"github.com/Adrient-tech/Adrient-Workspace/internal/video"
	"github.com/Adrient-tech/Adrient-Workspace/internal/vpn"
	"github.com/Adrient-tech/Adrient-Workspace/internal/whiteboards"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	version = "0.0.0-dev"
	commit  = "unknown"
)

func main() {
	version = buildinfo.Resolve(version, commit, "").Version // embedded VERSION unless -ldflags overrides
	httpAddr := flag.String("http-addr", ":8080", "HTTP/REST listen address")
	grpcAddr := flag.String("grpc-addr", ":9000", "gRPC listen address")
	dsn := flag.String("postgres-dsn", os.Getenv("ADRIENT_POSTGRES_DSN"), "Postgres DSN")
	staticDir := flag.String("static-dir", os.Getenv("ADRIENT_STATIC_DIR"), "Path to the built React SPA (web/app/dist). Empty = API-only.")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if *dsn == "" {
		logger.Error("postgres DSN is required (--postgres-dsn or ADRIENT_POSTGRES_DSN)")
		os.Exit(1)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer startupCancel()

	pool, err := storage.NewPool(startupCtx, *dsn)
	if err != nil {
		logger.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := storage.RunMigrations(startupCtx, pool); err != nil {
		logger.Error("run migrations", "err", err)
		os.Exit(1)
	}
	logger.Info("migrations applied")

	authCfg, err := loadAuthConfigFromEnv()
	if err != nil {
		logger.Error("load auth config", "err", err)
		os.Exit(1)
	}

	oidcClient, err := auth.NewOIDC(startupCtx, authCfg)
	if err != nil {
		logger.Error("init OIDC", "err", err)
		os.Exit(1)
	}

	orgsRepo := orgs.NewRepository(pool)
	defaultOrg, err := orgsRepo.GetBySlug(startupCtx, authCfg.DefaultOrgSlug)
	if err != nil {
		logger.Error("lookup default org", "err", err)
		os.Exit(1)
	}
	logger.Info("default org resolved", "id", defaultOrg.ID, "slug", defaultOrg.Slug)

	blobs, err := drive.NewBlobs(startupCtx, drive.BlobsConfig{
		Endpoint:  os.Getenv("ADRIENT_RUSTFS_ENDPOINT"),
		AccessKey: os.Getenv("ADRIENT_RUSTFS_ACCESS_KEY"),
		SecretKey: os.Getenv("ADRIENT_RUSTFS_SECRET_KEY"),
		Bucket:    defaultEnv("ADRIENT_RUSTFS_BUCKET", "adrient-default"),
		Region:    "us-east-1",
	})
	if err != nil {
		logger.Error("init drive blobs", "err", err)
		os.Exit(1)
	}

	// Per-object ACL grants (object_grants) — the per-user sharing primitive for
	// Drive + Docs. Shared by both services; nil notifier = no-op (best-effort).
	sharingRepo := sharing.NewRepository(pool)

	// Notifications feed — wired before Drive so the OnGrant hook can reference it.
	notifRepo := notifications.NewRepository(pool)

	// When a user is granted access to a shared object, emit a notification to
	// them. We use the OnGrant callback on sharingRepo to avoid a hard import
	// of internal/notifications in internal/sharing.
	sharingRepo.OnGrant = func(ctx context.Context, e sharing.GrantEvent) {
		_, _ = notifRepo.Create(ctx, notifications.CreateParams{
			// Notifications are scoped to the org the grantee belongs to; we do a
			// best-effort lookup inside the repo — if it fails the notification is
			// silently dropped rather than failing the share operation.
			OrgID:       resolveUserOrg(ctx, pool, e.GranteeUserID),
			UserID:      e.GranteeUserID,
			Type:        "share_grant",
			ActorUserID: e.GrantedBy,
			Title:       shareGrantTitle(e),
			Body:        "You have been granted " + e.Role + " access.",
			TargetURL:   shareGrantURL(e),
		})
	}

	// ---- Forgejo org auto-provisioning ----------------------------------------
	// When a adrient org is created, mirror it into Forgejo (best-effort, never
	// blocks org creation). Activated by ADRIENT_FORGEJO_URL + ADRIENT_FORGEJO_ADMIN_TOKEN;
	// both env vars must be set — if either is empty the provisioner is a no-op.
	//
	// Username mapping: the Forgejo username is the local-part of the creator's
	// adrient email (e.g. alice@example.com → "alice"). See internal/forgejo package
	// docs for the assumption and its limitations.
	forgejoProvisioner := forgejo.NewProvisionerFromEnv()
	orgsRepo.OnCreate = func(ctx context.Context, o orgs.Org, creatorEmail string) {
		forgejoProvisioner.OnOrgCreated(ctx, forgejo.OrgEvent{
			OrgID:        o.ID,
			Slug:         o.Slug,
			DisplayName:  o.DisplayName,
			CreatorEmail: creatorEmail,
		})
	}
	// ---- end Forgejo wiring ----------------------------------------------------

	driveSvc := drive.NewService(
		drive.NewRepository(pool),
		drive.NewACL(pool),
		blobs,
	).WithSharing(sharingRepo, nil)

	// Seed one sample book of each supported format into the default org's
	// library on first run (no-op when the library already has books).
	booksRepo := books.NewRepository(pool)
	books.SeedSamples(startupCtx, booksRepo, blobs, defaultOrg.ID)

	// Music radio: seed the built-in stations into the default org (no-op once
	// seeded), construct the stream recorder, and start the retention sweeper.
	// Recording is best-effort; failures never block startup or playback.
	musicRepo := music.NewRepository(pool)
	music.SeedStations(startupCtx, musicRepo, defaultOrg.ID)
	// The janitor enforces per-station retention plus the instance-wide radio
	// cache cap (ADRIENT_RADIO_CACHE_MAX_BYTES / _MAX_DAYS), and deletes the S3
	// objects + rows of trashed radio tracks. It runs hourly and is kicked
	// after every cached song.
	radioLimits := music.RadioCacheLimitsFromEnv()
	logger.Info("radio cache limits", "max_bytes", radioLimits.MaxBytes, "max_days", radioLimits.MaxDays)
	radioJanitor := radio.NewJanitor(musicRepo, blobs, radioLimits, time.Hour)
	radioJanitor.Start(context.Background())
	// CRM background work: idle-deal automation sweep and opted-in mail logging.
	crmRepo := crm.NewRepository(pool)
	go crmRepo.RunBackground(context.Background(), 5*time.Minute)
	musicRecorder := radio.NewRecorder(musicRepo, blobs).WithAfterSave(radioJanitor.Kick)

	// Feature flag: ADRIENT_PDF_BUILTIN=true mounts the PDF signing backend
	// in-process (gRPC + gateway + raw-HTTP under /pdf-api/), authenticated by
	// adrient's session via the auth bridge. Default (unset/anything-but-"true")
	// leaves the legacy /pdf-api reverse-proxy path untouched — behavior is
	// byte-for-byte identical to before this feature.
	// Feature flag: ADRIENT_LIBREOFFICE=1 enables the optional LibreOffice
	// headless converter (legacy .doc/.xls/.ppt import). Off by default; the
	// default image doesn't ship LibreOffice.
	officeConverter := convert.New(convert.ConfigFromEnv())
	if os.Getenv("ADRIENT_LIBREOFFICE") != "" {
		if officeConverter.Enabled() {
			logger.Info("libreoffice conversion enabled", "soffice", officeConverter.Binary())
		} else {
			logger.Warn("ADRIENT_LIBREOFFICE is set but no soffice binary was found (set ADRIENT_SOFFICE_PATH); legacy office import stays off")
		}
	}

	var pdfBuiltin *pdfapp.App
	if os.Getenv("ADRIENT_PDF_BUILTIN") == "true" {
		var perr error
		pdfBuiltin, perr = pdfapp.New(startupCtx, os.Environ())
		if perr != nil {
			// Fail OPEN: a PDF misconfiguration must never take down the whole
			// adrient app. Log loudly and continue with pdfBuiltin=nil, which
			// leaves /pdf-api on the legacy reverse-proxy path (PDF unavailable
			// rather than adrient crash-looping).
			logger.Error("ADRIENT_PDF_BUILTIN is on but PDF backend failed to construct; continuing without built-in PDF", "err", perr)
			pdfBuiltin = nil
		} else {
			defer pdfBuiltin.Close()
			logger.Info("PDF backend mounted in-process (ADRIENT_PDF_BUILTIN=true)")
		}
	}

	// On-demand container desktops (Guacamole Phase 2) — adrient.com only. Enabled
	// by ADRIENT_DESKTOPS_ENABLED; needs the in-cluster kube API + a Guacamole admin
	// credential. Disabled or not-in-cluster ⇒ a nil service (routes 404, no reaper).
	var desktopsSvc *desktops.Service
	if os.Getenv("ADRIENT_DESKTOPS_ENABLED") == "true" {
		ns := defaultEnv("ADRIENT_DESKTOPS_NAMESPACE", "adrient-desktops")
		if kube, kerr := desktops.NewInClusterKube(ns); kerr != nil {
			logger.Warn("desktops disabled: not running in-cluster", "err", kerr)
		} else {
			desktopsSvc = desktops.NewService(desktops.Config{
				Enabled:         true,
				VMsEnabled:      os.Getenv("ADRIENT_DESKTOP_VMS_ENABLED") == "true",
				Namespace:       ns,
				StorageClass:    defaultEnv("ADRIENT_DESKTOPS_STORAGE_CLASS", "ceph-block"),
				IdleTTL:         30 * time.Minute,
				MaxPerUser:      2,
				GuacOpenBaseURL: os.Getenv("ADRIENT_GUAC_URL"),
			},
				desktops.NewRepository(pool),
				kube,
				desktops.NewGuacClient(
					os.Getenv("ADRIENT_GUAC_API_URL"),
					os.Getenv("ADRIENT_GUAC_ADMIN_USER"),
					os.Getenv("ADRIENT_GUAC_ADMIN_PASSWORD"),
				),
			)
		}
	}

	// Domain-verification TXT override: development only. Needs BOTH the dev
	// flag and a file path, so a stray variable can never relax production.
	var txtResolver orgdomains.Resolver
	if os.Getenv("ADRIENT_DEV_MODE") == "1" && os.Getenv("ADRIENT_DEV_TXT_OVERRIDE") != "" {
		txtResolver = orgdomains.DevOverrideResolver{Path: os.Getenv("ADRIENT_DEV_TXT_OVERRIDE"), Next: orgdomains.NetResolver{}}
		slog.Warn("identity: DEV TXT override enabled", "file", os.Getenv("ADRIENT_DEV_TXT_OVERRIDE"))
	}

	// Talent (recruitment, onboarding, performance, learning), wired to Calendar, Meet, Drive, Tasks and mail.
	talentRepo := talent.NewWired(talent.WireConfig{Pool: pool, Blobs: blobs, Drive: drive.NewRepository(pool),
		Calendar: calendar.NewRepository(pool), Meet: meet.NewRepository(pool), EventMeet: eventmeet.NewRepository(pool),
		Tasks: tasks.NewRepository(pool), Mail: email.NewSenderFromEnv()})

	invoicingRepo := invoicing.NewRepository(pool)
	knowSet := know.New(know.Config{Invoicing: invoicingRepo, Pool: pool, Blobs: blobs, Drive: drive.NewRepository(pool), Calendar: calendar.NewRepository(pool), Tasks: tasks.NewRepository(pool), Contacts: contacts.NewRepository(pool), Mail: email.NewSenderFromEnv(), Notifications: notifications.NewRepository(pool)})

	srv := server.New(server.Config{
		Know:       knowSet,
		TalentRepo: talentRepo,
		HelpMail:   helpMailFunc(email.NewSenderFromEnv()),
		BookingRepo: booking.NewWired(booking.WireConfig{Pool: pool, Calendar: calendar.NewRepository(pool),
			Meet: meet.NewRepository(pool), EventMeet: eventmeet.NewRepository(pool), Mail: email.NewSenderFromEnv(),
			BaseURL: alexaPublicBaseURL()}),
		TXTResolver:       txtResolver,
		Version:           version,
		Commit:            commit,
		StartedAt:         time.Now(),
		AuthConfig:        authCfg,
		DesktopsService:   desktopsSvc,
		OIDC:              oidcClient,
		Sessions:          auth.NewSessionStore(pool),
		UsersRepo:         users.NewRepository(pool),
		OrgsRepo:          orgsRepo,
		DocsRepo:          docs.NewRepository(pool),
		SheetsRepo:        sheets.NewRepository(pool),
		SlidesRepo:        slides.NewRepository(pool),
		ContactsRepo:      contacts.NewRepository(pool),
		WhiteboardsRepo:   whiteboards.NewRepository(pool),
		CalendarRepo:      calendar.NewRepository(pool),
		MailRepo:          mail.NewRepository(pool),
		MailBackend:       mailBackend(),
		MailBlobs:         blobs,
		GamesRepo:         games.NewRepository(pool),
		GamesBlobs:        blobs,
		ChatRepo:          chat.NewRepository(pool),
		ChatBlobs:         blobs,
		MeetRepo:          meet.NewRepository(pool),
		TelephonyRepo:     telephony.NewRepository(pool),
		FormsRepo:         forms.NewRepository(pool),
		PhotosRepo:        photos.NewRepository(pool),
		PhotosBlobs:       blobs,
		BooksRepo:         booksRepo,
		BooksBlobs:        blobs,
		VideoRepo:         video.NewRepository(pool),
		VideoBlobs:        blobs,
		VideoShareRepo:    video.NewShareRepository(pool),
		VideoPlaylistRepo: video.NewPlaylistRepository(pool),
		VideoProgressRepo: video.NewProgressRepository(pool),
		VideoCaptionRepo:  video.NewCaptionRepository(pool),
		PublicHost:        os.Getenv("ADRIENT_PUBLIC_HOST"),
		OfficeConverter:   officeConverter,
		LiveRepo:          live.NewRepository(pool),
		LiveURLs: live.URLConfig{
			HLSBase:  defaultEnv("ADRIENT_LIVE_HLS_BASE", "/live-hls"),
			WHEPBase: defaultEnv("ADRIENT_LIVE_WEBRTC_BASE", "/live-webrtc"),
			WHIPBase: defaultEnv("ADRIENT_LIVE_WEBRTC_BASE", "/live-webrtc"),
			RTMPHost: defaultEnv("ADRIENT_LIVE_RTMP_HOST", "localhost:1935"),
		},
		LiveHLSURL:    defaultEnv("ADRIENT_LIVE_HLS_URL", "http://127.0.0.1:8888"),
		LiveWebRTCURL: defaultEnv("ADRIENT_LIVE_WEBRTC_URL", "http://127.0.0.1:8889"),
		MusicRepo:     musicRepo,
		MusicExtras:   music.NewExtras(pool, blobs),
		VideoExtras:   video.NewExtras(pool, blobs),
		MusicBlobs:    blobs,
		MusicRadio:    musicRecorder,
		PodcastsRepo:  podcasts.NewRepository(pool),
		Stories:       newStories(pool, blobs, notifRepo),
		// Self-hosted Alexa music skill. ADRIENT_ALEXA_SECRET signs the session-free
		// Echo stream URLs (no existing signing key to reuse, so it's its own env);
		// empty disables the skill. ADRIENT_PUBLIC_BASE_URL is the public origin used
		// to build absolute stream URLs (falls back to the OIDC redirect origin).
		// ADRIENT_ALEXA_SKIP_VERIFY=true bypasses request-signature checks (dev only).
		AlexaSecret:        []byte(os.Getenv("ADRIENT_ALEXA_SECRET")),
		AlexaPublicBaseURL: alexaPublicBaseURL(),
		AlexaSkipVerify:    boolEnv("ADRIENT_ALEXA_SKIP_VERIFY"),
		ProjectsRepo:       projects.NewRepository(pool),
		KeepRepo:           keep.NewRepository(pool),
		TasksRepo:          tasks.NewRepository(pool),
		InventoryRepo:      inventory.NewRepository(pool),
		InvoicingRepo:      invoicingRepo,
		AccountingRepo:     accounting.NewRepository(pool),
		CrmRepo:            crmRepo,
		HRRepo: hr.NewRepository(pool).
			WithCalendar(hr.CalendarAdapter{Repo: calendar.NewRepository(pool)}).
			WithNotifier(hr.NotificationsAdapter{Repo: notifRepo}).
			WithDocuments(blobs, hr.DriveAdapter{Pool: pool, Repo: drive.NewRepository(pool)}).
			WithAccounting(hr.AccountingAdapter{Repo: accounting.NewRepository(pool)}),
		PosRepo:            pos.NewRepository(pool, inventory.NewRepository(pool)),
		NotificationsRepo:  notifRepo,
		PrefsRepo:          prefs.NewRepository(pool),
		SitesRepo:          sites.NewRepository(pool),
		GroupsRepo:         groups.NewRepository(pool),
		SearchRepo:         search.NewRepository(pool),
		// AI assistant: provider keys are sealed with ADRIENT_SECRETS_KEY (or read from ADRIENT_AI_* env vars).
		AIGateway: ai.NewGateway(ai.NewPGStore(pool), os.Getenv("ADRIENT_SECRETS_KEY")),
		AccessRepo:         access.NewRepository(pool),
		GuacURL:            os.Getenv("ADRIENT_GUAC_URL"),
		AdminRepo:          admin.NewRepository(pool),
		AdminEmails:        os.Getenv("ADRIENT_ADMIN_EMAILS"),
		// Deployment-default external URLs for bring-your-own tiles; an org's own
		// Admin > Services URL always wins. The Helm chart sets
		// ADRIENT_HOMEASSISTANT_URL when it deploys Home Assistant itself.
		AdminDefaultURLs: map[string]string{
			admin.ServiceHomeAssistant: os.Getenv("ADRIENT_HOMEASSISTANT_URL"),
		},
		OrgAdminRepo:        orgadmin.NewRepository(pool),
		SharingRepo:         sharingRepo,
		IssuerURL:           authCfg.IssuerURL,
		AuditRepo:           audit.NewRepository(pool),
		BrandingRepo:        branding.NewRepository(pool),
		BrandingBlobs:       blobs,
		AvatarRepo:          useravatar.NewRepository(pool),
		AvatarBlobs:         blobs,
		SlidesBlobs:         blobs,
		DocsBlobs:           blobs,
		BrowserAccountStore: multiaccounts.NewStore(pool),
		ZitadelAPIURL:       os.Getenv("ADRIENT_ZITADEL_API_URL"),
		ZitadelServiceToken: os.Getenv("ADRIENT_ZITADEL_SERVICE_TOKEN"),
		// EmailSender is configured from RESEND_API_KEY + ADRIENT_EMAIL_FROM.
		// When RESEND_API_KEY is unset the sender is a no-op: invite emails are
		// logged but not delivered, so dev environments work without credentials.
		EmailSender: email.NewSenderFromEnv(),
		DefaultOrg:  defaultOrg,
		Drive:       driveSvc,
		Pool:        pool,
		StaticDir:   *staticDir,
		SiteName:    cmp.Or(os.Getenv("ADRIENT_SITE_NAME"), "Adrient Workspace"),
		DemoLogin:   loadDemoConfig(),
		// Integrated PDF (editor & sign) app proxy targets. Empty disables the
		// /pdf reverse proxy (e.g. when the PDF services aren't running).
		PDFFrontendURL: os.Getenv("ADRIENT_PDF_FRONTEND_URL"),
		PDFBackendURL:  os.Getenv("ADRIENT_PDF_BACKEND_URL"),
		// In-process PDF backend (ADRIENT_PDF_BUILTIN). nil = legacy reverse-proxy.
		PDFBuiltin: pdfBuiltin,
		// In-process PDF frontend SPA (Phase 2c). When set AND the built-in PDF
		// backend is on, adrient serves the PDF SPA from this dir for /pdf*.
		// Empty = legacy PDFFrontendURL reverse-proxy.
		PDFStaticDir: os.Getenv("ADRIENT_PDF_STATIC_DIR"),
		// Integrated Twenty CRM. ADRIENT_CRM_URL is Twenty's internal origin;
		// ADRIENT_CRM_HOST is the public subdomain whose requests get proxied
		// whole to Twenty at root. Both empty disables the CRM proxy.
		CRMURL:  os.Getenv("ADRIENT_CRM_URL"),
		CRMHost: os.Getenv("ADRIENT_CRM_HOST"),
		// Orona Bolo multiplayer server origin. adrient reverse-proxies /bolo-mp/*
		// to it (WebSocket-capable), stripping the prefix. Empty disables the
		// proxy (the bolo single-player path is unaffected). e.g. http://127.0.0.1:6173.
		BoloMpURL: os.Getenv("ADRIENT_BOLO_MP_URL"),
		// Per-instance Forgejo (git hosting) reverse-proxied at /git/* (prefix
		// stripped). Set Forgejo ROOT_URL to https://<host>/git/. Empty disables.
		ForgejoURL: os.Getenv("ADRIENT_FORGEJO_URL"),
		// Same provisioner instance used for org-creation mirroring; also drives
		// access-time /git SSO org/team provisioning. No-op when unconfigured.
		ForgejoProvisioner: forgejoProvisioner,
		// Shared HMAC secret for inbound Forgejo webhooks (server-to-server,
		// HMAC-verified, bypasses session auth). Empty disables the endpoint.
		ForgejoWebhookSecret: os.Getenv("ADRIENT_FORGEJO_WEBHOOK_SECRET"),
		// Assemble (spatial collaboration) reverse-proxied at /assemble/* (prefix
		// stripped, auth-wall bypassed so guests work). Empty disables.
		AssembleURL: os.Getenv("ADRIENT_ASSEMBLE_URL"),
		// Learn platform (Learn PWA) reverse-proxied at /learn/* (prefix
		// stripped, auth-wall bypassed — Learn is public). Empty disables.
		LearnURL: os.Getenv("ADRIENT_LEARN_URL"),
		// Cloud Import — upload-based import from Google Takeout and Apple exports.
		CloudImportRepo:      cloudimport.NewRepository(pool),
		CloudImportDriveRepo: drive.NewRepository(pool),
		CloudImportBlobs:     blobs,
		// VPN (Tailscale) status endpoint.
		VPNHandler: vpn.NewHandlerFromEnv(),
	})

	httpServer := &http.Server{
		Addr:              *httpAddr,
		Handler:           srv.HTTPHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	grpcLis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		logger.Error("listen gRPC", "err", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("serving gRPC", "addr", *grpcAddr)
		if err := srv.GRPC().Serve(grpcLis); err != nil && !errors.Is(err, net.ErrClosed) {
			logger.Error("gRPC serve", "err", err)
		}
	}()

	go func() {
		logger.Info("serving HTTP", "addr", *httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP serve", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	srv.GRPC().GracefulStop()
}

// mailBackend selects the mail backend. Default (or ADRIENT_MAIL_BACKEND=local)
// is the Postgres store with internal delivery. ADRIENT_MAIL_BACKEND=imap (or
// "mailcow") uses the IMAP/SMTP bridge configured via ADRIENT_MAIL_* env vars.
func mailBackend() mail.Backend {
	switch os.Getenv("ADRIENT_MAIL_BACKEND") {
	case "imap", "mailcow":
		return mail.NewBridge(mail.BridgeConfigFromEnv())
	default:
		return nil // server falls back to the Postgres LocalBackend
	}
}

// loadAuthConfigFromEnv reads ADRIENT_OIDC_* env vars into an auth.Config.
func loadAuthConfigFromEnv() (auth.Config, error) {
	secure, _ := strconv.ParseBool(os.Getenv("ADRIENT_SESSION_COOKIE_SECURE"))
	lifetimeStr := os.Getenv("ADRIENT_SESSION_LIFETIME")
	if lifetimeStr == "" {
		lifetimeStr = "168h"
	}
	lifetime, err := time.ParseDuration(lifetimeStr)
	if err != nil {
		return auth.Config{}, err
	}
	cfg := auth.Config{
		IssuerURL:       os.Getenv("ADRIENT_OIDC_ISSUER"),
		ClientID:        os.Getenv("ADRIENT_OIDC_CLIENT_ID"),
		ClientSecret:    os.Getenv("ADRIENT_OIDC_CLIENT_SECRET"),
		RedirectURL:     os.Getenv("ADRIENT_OIDC_REDIRECT_URL"),
		CookieName:      defaultEnv("ADRIENT_SESSION_COOKIE_NAME", "adrient_session"),
		CookieSecure:    secure,
		CookieDomain:    sessionCookieDomain(),
		SessionLifetime: lifetime,
		DefaultOrgSlug:  defaultEnv("ADRIENT_DEFAULT_ORG_SLUG", "default"),
		PersonalOrgs:    personalOrgsEnabled(),
	}
	return cfg, cfg.Validate()
}

// sessionCookieDomain decides the Domain attribute for the session + OIDC-state
// cookies. An explicit ADRIENT_SESSION_COOKIE_DOMAIN wins. Otherwise it is derived
// from the OIDC redirect URL's host so the cookies are valid across subdomains
// (e.g. the CRM subdomain crm.<host>) with no extra config. Hosts that can't
// carry a Domain attribute (bare "localhost", IP addresses) stay host-only.
func sessionCookieDomain() string {
	if d := os.Getenv("ADRIENT_SESSION_COOKIE_DOMAIN"); d != "" {
		return d
	}
	u, err := url.Parse(os.Getenv("ADRIENT_OIDC_REDIRECT_URL"))
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return "" // localhost / IP — cannot scope to a parent domain
	}
	return host
}

// personalOrgsEnabled reports whether first-ever sign-ins get a personal
// (org-per-user) org. Controlled by ADRIENT_PERSONAL_ORGS; defaults to ON. Set it
// to a falsey value ("0", "false", "off", "no") to keep all new users in the
// shared default org (legacy single-org behavior).
func personalOrgsEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ADRIENT_PERSONAL_ORGS")))
	switch v {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// loadDemoConfig reads ADRIENT_DEMO_LOGIN_ENABLED and ADRIENT_DEMO_USERNAME from the
// environment. ADRIENT_DEMO_PASSWORD is intentionally not read here — it lives only
// in the operator's secrets store and in Zitadel; adrient never handles it.
func loadDemoConfig() auth.DemoConfig {
	enabled, _ := strconv.ParseBool(os.Getenv("ADRIENT_DEMO_LOGIN_ENABLED"))
	return auth.DemoConfig{
		Enabled:  enabled,
		Username: os.Getenv("ADRIENT_DEMO_USERNAME"),
	}
}

func defaultEnv(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// boolEnv parses a boolean env var; anything unparseable (incl. empty) is false.
func boolEnv(name string) bool {
	b, _ := strconv.ParseBool(os.Getenv(name))
	return b
}

// alexaPublicBaseURL returns adrient's public origin used to build absolute,
// signed Alexa stream URLs. An explicit ADRIENT_PUBLIC_BASE_URL wins; otherwise it
// is derived from the OIDC redirect URL's scheme+host (e.g. https://adrient.com).
func alexaPublicBaseURL() string {
	if v := os.Getenv("ADRIENT_PUBLIC_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	u, err := url.Parse(os.Getenv("ADRIENT_OIDC_REDIRECT_URL"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// resolveUserOrg looks up the org_id for a given user id. Returns "" on any
// error so the caller can skip the notification silently.
func resolveUserOrg(ctx context.Context, pool *pgxpool.Pool, userID string) string {
	if userID == "" {
		return ""
	}
	var orgID string
	_ = pool.QueryRow(ctx, `SELECT org_id::text FROM adrient.users WHERE id=$1`, userID).Scan(&orgID)
	return orgID
}

// shareGrantTitle builds a human-readable notification title for a sharing grant.
func shareGrantTitle(e sharing.GrantEvent) string {
	switch e.ObjectType {
	case sharing.TypeDriveFile:
		return "A file was shared with you"
	case sharing.TypeDocsDoc:
		return "A document was shared with you"
	case sharing.TypeSheetsSheet:
		return "A spreadsheet was shared with you"
	case sharing.TypeSlidesDeck:
		return "A presentation was shared with you"
	default:
		return "An item was shared with you"
	}
}

// shareGrantURL builds a deep link for the notification based on the object type.
func shareGrantURL(e sharing.GrantEvent) string {
	switch e.ObjectType {
	case sharing.TypeDriveFile:
		return "/drive"
	case sharing.TypeDocsDoc:
		return "/docs/d/" + e.ObjectID
	case sharing.TypeSheetsSheet:
		return "/sheets/d/" + e.ObjectID
	case sharing.TypeSlidesDeck:
		return "/slides/d/" + e.ObjectID
	default:
		return "/"
	}
}

// helpMailFunc adapts the email sender for the help desk and forms; nil when
// mail is not configured, so those features say so instead of failing silently.
func helpMailFunc(m *email.Sender) func(ctx context.Context, to []string, subject, htmlBody, text string) error {
	if m == nil || !m.Configured() {
		return nil
	}
	return func(ctx context.Context, to []string, subject, htmlBody, text string) error {
		return m.Send(ctx, email.Message{To: to, Subject: subject, HTML: htmlBody, Text: text})
	}
}

// newStories builds the Stories service (audio generation worker and scheduler included).
func newStories(pool *pgxpool.Pool, blobs *drive.Blobs, n *notifications.Repository) *podcasts.Stories {
	st := podcasts.NewStories(pool, blobs)
	st.Secret = os.Getenv("ADRIENT_SECRETS_KEY")
	st.Notify = func(ctx context.Context, orgID, userID, actorID, typ, title, body, url string) {
		if _, err := n.Create(ctx, notifications.CreateParams{OrgID: orgID, UserID: userID, ActorUserID: actorID, Type: typ, Title: title, Body: body, TargetURL: url}); err != nil {
			slog.Warn("stories: notification", "err", err)
		}
	}
	st.Start()
	return st
}
