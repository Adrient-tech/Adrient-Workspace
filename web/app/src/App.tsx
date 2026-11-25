import { useEffect, useState, lazy, Suspense } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import { CssVarsProvider, CssBaseline } from "@mui/joy";

import { adrientTheme } from "./theme";
import { BrandProvider } from "./brand/Brand";
import { PublicBrandLine } from "./brand/ProductBadge";
import { AppSplash } from "./components/AppSplash";
import { KeyboardHelpOverlay } from "./components/KeyboardHelpOverlay";
import { whoami } from "./api/client";
import type { User } from "./api/types";
import { Dashboard } from "./pages/Dashboard";
import { Home } from "./pages/home/Home";
import { Shell } from "./components/Shell";
import { SignIn } from "./pages/SignIn";
import { ComingSoon } from "./pages/ComingSoon";
import { NotFound } from "./pages/NotFound";
import { FileList } from "./pages/drive/FileList";
import { FileViewer } from "./pages/drive/FileViewer";
import { EditorPlaceholder } from "./pages/EditorPlaceholder";
import { useActiveCall } from "./pages/meet/callStore";
import { AppPaneProvider } from "./components/AppPane";
import { LinkHandler } from "./components/LinkHandler";
import { isEmbedded } from "./lib/embed";
import { LiveCenter } from "./components/LiveCenter";
const CallHost = lazy(() => import("./pages/meet/CallHost").then((m) => ({ default: m.CallHost })));

// Docs ships as its own code-split chunk (TipTap + Yjs) loaded on demand.
const DocsApp = lazy(() => import("./pages/docs"));
// Public share route: openable without an account, in any auth state.
const SharedDoc = lazy(() => import("./pages/docs/SharedDoc"));
// Public video watch route: openable without an account.
const VideoWatch = lazy(() => import("./pages/video/VideoWatch"));
// Sheets ships as its own chunk (FortuneSheet).
const SheetsApp = lazy(() => import("./pages/sheets"));
const SlidesApp = lazy(() => import("./pages/slides"));
const ContactsApp = lazy(() => import("./pages/contacts"));
const WhiteboardApp = lazy(() => import("./pages/whiteboard"));
const OrgChartPage = lazy(() => import("./pages/orgchart"));
const CalendarApp = lazy(() => import("./pages/calendar"));
const MailApp = lazy(() => import("./pages/mail"));
const ChatApp = lazy(() => import("./pages/chat"));
const MeetApp = lazy(() => import("./pages/meet"));
const TelephonyApp = lazy(() => import("./pages/telephony"));
const OrgSyncApp = lazy(() => import("./pages/orgsync"));
const FormsApp = lazy(() => import("./pages/forms"));
const PhotosApp = lazy(() => import("./pages/photos"));
const BooksApp = lazy(() => import("./pages/books"));
const VideoApp = lazy(() => import("./pages/video"));
const LiveApp = lazy(() => import("./pages/live"));
const LiveWatchPublic = lazy(() => import("./pages/live/WatchStream"));
const GamesApp = lazy(() => import("./pages/games"));
const MusicApp = lazy(() => import("./pages/music"));
const ProjectsApp = lazy(() => import("./pages/projects"));
const AcceptInvite = lazy(() => import("./pages/AcceptInvite"));
const AdminApp = lazy(() => import("./pages/admin"));
const KeepApp = lazy(() => import("./pages/keep"));
const TasksApp = lazy(() => import("./pages/tasks"));
const InventoryApp = lazy(() => import("./pages/inventory"));
const PosApp = lazy(() => import("./pages/pos"));
const InvoicingApp = lazy(() => import("./pages/invoicing"));
const AccountingApp = lazy(() => import("./pages/accounting"));
const CrmApp = lazy(() => import("./pages/crm"));
const TalentApp = lazy(() => import("./pages/talent"));
const RecordsApp = lazy(() => import("./pages/records"));
const WikiApp = lazy(() => import("./pages/wiki"));
const ContractsApp = lazy(() => import("./pages/contracts"));
const LegalApp = lazy(() => import("./pages/legal"));
const CapTableApp = lazy(() => import("./pages/captable"));
const SignaturesApp = lazy(() => import("./pages/signatures"));
const SignPage = lazy(() => import("./pages/signatures/Sign"));
const PublicBookingPage = lazy(() => import("./pages/booking/PublicBooking").then((m) => ({ default: m.PublicBookingPage })));
const CancelBookingPage = lazy(() => import("./pages/booking/PublicBooking").then((m) => ({ default: m.CancelBookingPage })));
const HelpCenterPage = lazy(() => import("./pages/tickets/PublicHelp").then((m) => ({ default: m.HelpCenterPage })));
const PortalPage = lazy(() => import("./pages/tickets/Portal").then((m) => ({ default: m.PortalPage })));
const TrackPage = lazy(() => import("./pages/tickets/Portal").then((m) => ({ default: m.TrackPage })));
const CsatPage = lazy(() => import("./pages/tickets/PublicHelp").then((m) => ({ default: m.CsatPage })));
const CareersPage = lazy(() => import("./pages/talent/Careers").then((m) => ({ default: m.CareersPage })));
const OfferPage = lazy(() => import("./pages/talent/Careers").then((m) => ({ default: m.OfferPage })));
const HrApp = lazy(() => import("./pages/hr"));
const ShiftsApp = lazy(() => import("./pages/shifts"));
const TimeTrackingApp = lazy(() => import("./pages/timetracking"));
const FieldServiceApp = lazy(() => import("./pages/fieldservice"));
const FleetApp = lazy(() => import("./pages/fleet"));
// Public invoice/estimate share page: openable without an account.
const PublicInvoice = lazy(() => import("./pages/invoicing/Public"));
const CustomerPortal = lazy(() => import("./pages/invoicing/Portal"));
const PosDisplay = lazy(() => import("./pages/pos/Display"));
const SitesApp = lazy(() => import("./pages/sites"));
const SiteView = lazy(() => import("./pages/sites/SiteView"));
const GroupsApp = lazy(() => import("./pages/groups"));
const SettingsPage = lazy(() => import("./pages/settings"));
const SoundsPage = lazy(() => import("./pages/settings/Sounds"));
const MySecurityPage = lazy(() => import("./pages/security"));
const CloudImportApp = lazy(() => import("./pages/cloudimport"));
const VPNApp = lazy(() => import("./pages/vpn"));
const AccessPage = lazy(() => import("./pages/access"));
// Home Assistant: the org's HA framed under the header (bring-your-own URL).
const HomeAssistantApp = lazy(() => import("./pages/homeassistant"));
const TicketsApp = lazy(() => import("./pages/tickets"));
// 3D model viewer (three.js) — code-split so its loaders load on demand.
const ThreeDApp = lazy(() => import("./pages/3d"));
// Translate ships as its own chunk; the ML libs (transformers.js,
// onnxruntime-web) are dynamically imported inside the page on first use.
const TranslateApp = lazy(() => import("./pages/translate"));
// Maps ships as its own chunk (Leaflet + its CSS live in this lazy route).
const MapsApp = lazy(() => import("./pages/maps"));
// Stories (the Podcasts tile) — its own chunk.
const PodcastsApp = lazy(() => import("./pages/podcasts"));
// Archaeology — Leaflet + live Wikidata, its own chunk.
const ArchaeologyApp = lazy(() => import("./pages/archaeology"));
// Public ticket intake: file a request without an account.
const TicketSubmitPublic = lazy(() => import("./pages/tickets/Submit"));

function ChunkFallback() {
  return <AppSplash />;
}

type AuthState =
  | { kind: "loading" }
  | { kind: "authenticated"; user: User; isPersonalOrg: boolean }
  | { kind: "unauthenticated" }
  | { kind: "error"; message: string };

export default function App() {
  const [auth, setAuth] = useState<AuthState>({ kind: "loading" });
  const activeCall = useActiveCall();

  useEffect(() => {
    let cancelled = false;
    whoami().then((r) => {
      if (cancelled) return;
      switch (r.status) {
        case "ok":
          setAuth({
            kind: "authenticated",
            user: r.data.user,
            isPersonalOrg: !!r.data.org.is_personal,
          });
          break;
        case "unauthenticated":
          setAuth({ kind: "unauthenticated" });
          break;
        case "error":
          setAuth({ kind: "error", message: r.message });
          break;
      }
    });
    return () => {
      cancelled = true;
    };
  }, []);

  // Update avatar_url on the in-memory user when the Settings page uploads one.
  useEffect(() => {
    const handler = (e: Event) => {
      const { avatar_url } = (e as CustomEvent<{ avatar_url: string | null }>)
        .detail;
      setAuth((prev) => {
        if (prev.kind !== "authenticated") return prev;
        return {
          ...prev,
          user: { ...prev.user, avatar_url: avatar_url ?? undefined },
        };
      });
    };
    window.addEventListener("avatar-changed", handler);
    return () => window.removeEventListener("avatar-changed", handler);
  }, []);

  const shareRoute = (
    <Route
      path="/docs/share/:token"
      element={
        <Suspense fallback={<ChunkFallback />}>
          <SharedDoc />
          <PublicBrandLine />
        </Suspense>
      }
    />
  );

  // Published Sites pages are public — viewable without an account.
  const sitesViewRoute = (
    <Route
      path="/sites/view/:siteId"
      element={
        <Suspense fallback={<ChunkFallback />}>
          <SiteView />
        </Suspense>
      }
    />
  );

  // Public video share-link watch route.
  const videoWatchRoute = (
    <Route
      path="/video/watch/:token"
      element={
        <Suspense fallback={<ChunkFallback />}>
          <VideoWatch />
        </Suspense>
      }
    />
  );

  // Public live-stream watch — available signed-out for `public` streams.
  const liveWatchPublicRoute = (
    <Route
      path="/live/p/:id"
      element={
        <Suspense fallback={<ChunkFallback />}>
          <LiveWatchPublic user={null} publicRoute />
        </Suspense>
      }
    />
  );

  // Public ticket intake — anyone with a project's public link can file a
  // request, no account needed.
  const ticketSubmitRoute = (
    <Route
      path="/tickets/submit/:token"
      element={
        <Suspense fallback={<ChunkFallback />}>
          <TicketSubmitPublic />
        </Suspense>
      }
    />
  );

  // Public help centre and satisfaction survey (no account needed).
  const helpRoutes = [
    <Route key="help-article" path="/help/:token/:slug" element={<Suspense fallback={<ChunkFallback />}><HelpCenterPage /><PublicBrandLine /></Suspense>} />,
    <Route key="help" path="/help/:token" element={<Suspense fallback={<ChunkFallback />}><HelpCenterPage /><PublicBrandLine /></Suspense>} />,
    <Route key="portal" path="/support/:token" element={<Suspense fallback={<ChunkFallback />}><PortalPage /><PublicBrandLine /></Suspense>} />,
    <Route key="track" path="/support/:token/track/:trk" element={<Suspense fallback={<ChunkFallback />}><TrackPage /><PublicBrandLine /></Suspense>} />,
    <Route key="csat" path="/csat/:token" element={<Suspense fallback={<ChunkFallback />}><CsatPage /><PublicBrandLine /></Suspense>} />,
  ];

  // Public booking page and guest cancel page (no account needed).
  const bookingRoutes = [
    <Route key="book-cancel" path="/book/cancel/:token" element={<Suspense fallback={<ChunkFallback />}><CancelBookingPage /><PublicBrandLine /></Suspense>} />,
    <Route key="book" path="/book/:slug" element={<Suspense fallback={<ChunkFallback />}><PublicBookingPage /><PublicBrandLine /></Suspense>} />,
  ];

  // Public e-signature page for a signer's private link (no account needed).
  const signRoute = <Route key="sign" path="/sign/:token" element={<Suspense fallback={<ChunkFallback />}><SignPage /><PublicBrandLine /></Suspense>} />;

  // Public career page and offer e-accept (no account needed).
  const careersRoutes = [
    <Route key="careers-offer" path="/careers/offer/:token" element={<Suspense fallback={<ChunkFallback />}><OfferPage /><PublicBrandLine /></Suspense>} />,
    <Route key="careers" path="/careers/:org" element={<Suspense fallback={<ChunkFallback />}><CareersPage /><PublicBrandLine /></Suspense>} />,
  ];

  // Public invoice share page (/invoice/:token), read-only, no account needed.
  const invoiceShareRoute = (
    <>
      <Route
        path="/invoice/:token"
        element={
          <Suspense fallback={<ChunkFallback />}>
            <PublicInvoice />
            <PublicBrandLine />
          </Suspense>
        }
      />
      <Route
        path="/portal/:token"
        element={
          <Suspense fallback={<ChunkFallback />}>
            <CustomerPortal />
            <PublicBrandLine />
          </Suspense>
        }
      />
    </>
  );

  // Invitation landing page (/accept-invite?code=...): public, adapts to sign-in state.
  const acceptInviteRoute = (
    <Route
      path="/accept-invite"
      element={
        <Suspense fallback={<ChunkFallback />}>
          <AcceptInvite signedInEmail={auth.kind === "authenticated" ? auth.user.email : null} />
        </Suspense>
      }
    />
  );

  // Games are public — playable without an account, sign-in optional. The
  // page adapts to a null user when signed out.
  const gamesRoute = (
    <Route
      path="/games/*"
      element={
        <Suspense fallback={<ChunkFallback />}>
          <GamesApp user={auth.kind === "authenticated" ? auth.user : null} />
        </Suspense>
      }
    />
  );

  // Embedded panes leave the header, call bar and live toasts to the parent.
  const embedded = isEmbedded();

  return (
    <CssVarsProvider theme={adrientTheme} defaultMode="light">
      <CssBaseline />
      <BrandProvider>
       <AppPaneProvider>
        <LinkHandler />
        {auth.kind === "authenticated" && !embedded && <KeyboardHelpOverlay />}
        {auth.kind === "loading" && <AppSplash />}
        {auth.kind !== "loading" && (
          <Shell enabled={auth.kind === "authenticated" && !embedded}>
          <Routes>
            {/* Public — available even when signed out. More specific than
                /docs/*, so react-router ranks it ahead. */}
            {shareRoute}
            {sitesViewRoute}
            {videoWatchRoute}
            {liveWatchPublicRoute}
            {ticketSubmitRoute}
            {invoiceShareRoute}
            {careersRoutes}
            {signRoute}
            {bookingRoutes}
            {helpRoutes}
            <Route
              path="/pos/display"
              element={
                <Suspense fallback={<ChunkFallback />}>
                  <PosDisplay />
                </Suspense>
              }
            />
            {gamesRoute}
            {acceptInviteRoute}
            {auth.kind === "authenticated" ? (
              <>
                <Route path="/" element={<Home user={auth.user} />} />
                <Route path="/apps" element={<Dashboard user={auth.user} />} />
                <Route
                  path="/docs/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <DocsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/sheets/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <SheetsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/slides/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <SlidesApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/contacts/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <ContactsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/whiteboard/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <WhiteboardApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/calendar/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <CalendarApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/mail/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <MailApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/chat/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <ChatApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/meet/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <MeetApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/telephony/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <TelephonyApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/orgsync"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <OrgSyncApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/forms/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <FormsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/photos/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <PhotosApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/books/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <BooksApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/video/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <VideoApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/live/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <LiveApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/music/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <MusicApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/projects/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <ProjectsApp user={auth.user} />
                    </Suspense>
                  }
                />
                {/* Admin is unreachable in a single-user (personal) org — redirect
                    to the dashboard. Team orgs render it (admin-gated server-side). */}
                <Route
                  path="/admin"
                  element={
                    auth.isPersonalOrg ? (
                      <Navigate to="/" replace />
                    ) : (
                      <Suspense fallback={<ChunkFallback />}>
                        <AdminApp user={auth.user} />
                      </Suspense>
                    )
                  }
                />
                <Route
                  path="/admin/:section"
                  element={
                    auth.isPersonalOrg ? (
                      <Navigate to="/" replace />
                    ) : (
                      <Suspense fallback={<ChunkFallback />}>
                        <AdminApp user={auth.user} />
                      </Suspense>
                    )
                  }
                />
                <Route
                  path="/keep/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <KeepApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/tasks/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <TasksApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/inventory/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <InventoryApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/talent/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <TalentApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/fleet/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <FleetApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/records/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <RecordsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/fieldservice/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <FieldServiceApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/wiki/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <WikiApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/timetracking/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <TimeTrackingApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/legal/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <LegalApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/captable/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <CapTableApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/contracts/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <ContractsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/shifts/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <ShiftsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/signatures/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <SignaturesApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/hr/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <HrApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/crm/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <CrmApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/accounting/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <AccountingApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/invoicing/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <InvoicingApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/pos/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <PosApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/sites/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <SitesApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/groups/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <GroupsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/settings/sounds"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <SoundsPage user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/security"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <MySecurityPage user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/settings"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <SettingsPage user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/cloudimport/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <CloudImportApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/vpn"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <VPNApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/homeassistant"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <HomeAssistantApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/home-assistant"
                  element={<Navigate to="/homeassistant" replace />}
                />
                <Route
                  path="/access"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <AccessPage user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/tickets"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <TicketsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/coming-soon/:appId"
                  element={<ComingSoon user={auth.user} />}
                />
                <Route
                  path="/3d"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <ThreeDApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/translate"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <TranslateApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/maps"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <MapsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route path="/stories/*" element={<Navigate to="/podcasts" replace />} />
                <Route
                  path="/podcasts/*"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <PodcastsApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route
                  path="/archaeology"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <ArchaeologyApp user={auth.user} />
                    </Suspense>
                  }
                />
                <Route path="/drive" element={<FileList user={auth.user} />} />
                <Route
                  path="/drive/file/:id"
                  element={<FileViewer user={auth.user} />}
                />
                {/* Per-type editor placeholders from Drive's file-open flow. The
                    full Sheets/Slides/PDF editors aren't built yet; the doc-type
                    placeholder coexists with the real Docs app at /docs/*. */}
                <Route
                  path="/sheets/:id"
                  element={
                    <EditorPlaceholder user={auth.user} appId="sheets" />
                  }
                />
                <Route
                  path="/docs/:id"
                  element={<EditorPlaceholder user={auth.user} appId="docs" />}
                />
                <Route
                  path="/slides/:id"
                  element={
                    <EditorPlaceholder user={auth.user} appId="slides" />
                  }
                />
                <Route
                  path="/pdf/:id"
                  element={<EditorPlaceholder user={auth.user} appId="pdf" />}
                />
                <Route
                  path="/whiteboard/:id"
                  element={
                    <EditorPlaceholder user={auth.user} appId="whiteboard" />
                  }
                />
                <Route
                  path="/org-chart"
                  element={
                    <Suspense fallback={<ChunkFallback />}>
                      <OrgChartPage user={auth.user} />
                    </Suspense>
                  }
                />
                <Route path="/sign-in" element={<Navigate to="/" replace />} />
                <Route path="*" element={<NotFound />} />
              </>
            ) : auth.kind === "error" ? (
              <Route
                path="*"
                element={
                  <div role="alert" style={{ padding: 24 }}>
                    Error contacting backend: {auth.message}
                  </div>
                }
              />
            ) : (
              <Route path="*" element={<SignIn />} />
            )}
          </Routes>
          </Shell>
        )}
        {auth.kind === "authenticated" && !embedded && <LiveCenter />}
        {auth.kind === "authenticated" && !embedded && activeCall && (
          <Suspense fallback={null}>
            <CallHost />
          </Suspense>
        )}
       </AppPaneProvider>
      </BrandProvider>
    </CssVarsProvider>
  );
}
