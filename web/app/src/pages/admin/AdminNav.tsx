import { useMemo, useState, type ReactNode } from "react";
import { Box, Breadcrumbs, Input, Link, Typography } from "@mui/joy";
import SearchIcon from "@mui/icons-material/Search";

export interface NavItem {
  id: string;
  label: string;
  icon: ReactNode;
  enabled: boolean;
}

/** Group of every section. Unknown ids land in "More". */
export const NAV_GROUPS: { title: string; ids: string[] }[] = [
  { title: "Overview", ids: ["dashboard"] },
  { title: "People", ids: ["users", "people", "groups", "identity", "offboarding", "seats"] },
  { title: "Access", ids: ["roles", "adminroles", "appaccess", "controls", "effective", "reviews", "external"] },
  { title: "Security", ids: ["security", "signin", "sessions", "geo", "honeypot", "ratelimit"] },
  { title: "Apps", ids: ["services", "ai", "notifysources"] },
  { title: "Data and logs", ids: ["audit", "auditsearch", "analytics"] },
  { title: "Organisation", ids: ["settings", "domains", "provisioning"] },
];

/** One line of help per page, shown under the page title. Also searched by "Find a setting". */
export const SECTION_INFO: Record<string, { desc: string; keywords: string }> = {
  dashboard: { desc: "Health of your organisation at a glance: people, storage, sign-ins and things that need attention.", keywords: "overview home health alerts" },
  users: { desc: "Create, suspend and manage the people in your organisation.", keywords: "add user invite password reset suspend delete account" },
  people: { desc: "Departments, job titles and who reports to whom.", keywords: "department manager title org chart directory" },
  groups: { desc: "Groups share access and are used in app controls and mailing.", keywords: "team members distribution list" },
  roles: { desc: "Roles bundle permissions you can assign to people and groups.", keywords: "permissions admin role" },
  appaccess: { desc: "Choose which people and groups can open each app.", keywords: "enable disable app access permissions" },
  controls: { desc: "Per-app switches such as sharing, external links and downloads.", keywords: "sharing link download external policy toggle" },
  ai: { desc: "Turn the AI assistant on or off and choose what it may do.", keywords: "assistant model copilot" },
  effective: { desc: "See exactly what one person can open and why.", keywords: "preview access who can" },
  external: { desc: "Control sharing with people outside your organisation.", keywords: "guest share external domain" },
  domains: { desc: "Add and verify the domains your organisation owns.", keywords: "dns txt verify email domain" },
  identity: { desc: "Invite guests and manage pending invitations.", keywords: "guest invite invitation" },
  sessions: { desc: "See active sign-ins and end sessions you do not recognise.", keywords: "devices login revoke sign out" },
  audit: { desc: "A record of who did what, and when.", keywords: "log activity history events" },
  analytics: { desc: "Usage and adoption by app over time.", keywords: "usage adoption statistics reports" },
  security: { desc: "Organisation-wide security settings.", keywords: "mfa password policy two-step" },
  geo: { desc: "Allow or block sign-in by country.", keywords: "region country block allow geo" },
  honeypot: { desc: "Decoy accounts that raise an alert when someone probes them.", keywords: "trap decoy alert" },
  ratelimit: { desc: "Limits that slow down abuse of the sign-in and API.", keywords: "throttle brute force limit" },
  settings: { desc: "Organisation name, branding and defaults.", keywords: "name logo brand colour timezone language" },
  services: { desc: "Switch whole apps on or off for the organisation.", keywords: "enable disable app service" },
  provisioning: { desc: "Connect an identity provider and automate account creation.", keywords: "sso scim saml oidc sync" },
  seats: { desc: "Licences in use and how many are left.", keywords: "billing licence seat plan" },
  signin: { desc: "Rules for how people sign in.", keywords: "mfa two-step password" },
  offboarding: { desc: "Steps that run when someone leaves.", keywords: "leaver transfer files suspend" },
  reviews: { desc: "Periodic checks that people still need their access.", keywords: "recertification campaign review" },
  adminroles: { desc: "Who can administer the organisation, and what parts.", keywords: "delegated admin" },
  auditsearch: { desc: "Search the audit log by person, action and date.", keywords: "find filter export" },
  notifysources: { desc: "Apps and services allowed to send notifications to people.", keywords: "webhook alert source" },
};

export function groupOf(id: string): string {
  return NAV_GROUPS.find((g) => g.ids.includes(id))?.title ?? "More";
}

/** Match a "Find a setting" query against labels, descriptions and keywords. */
export function findSettings(items: { id: string; label: string }[], q: string): { id: string; label: string; group: string }[] {
  const words = q.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return [];
  return items
    .map((i) => {
      const info = SECTION_INFO[i.id];
      const hay = `${i.label} ${info?.desc ?? ""} ${info?.keywords ?? ""} ${groupOf(i.id)}`.toLowerCase();
      const score = words.reduce((s, w) => s + (i.label.toLowerCase().includes(w) ? 3 : hay.includes(w) ? 1 : -100), 0);
      return { id: i.id, label: i.label, group: groupOf(i.id), score };
    })
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, 12);
}

export function AdminSidebar({ items, active, onPick, onAfterPick }: { items: NavItem[]; active: string; onPick: (id: string) => void; onAfterPick?: () => void }) {
  const [q, setQ] = useState("");
  const hits = useMemo(() => findSettings(items, q), [items, q]);
  const byId = useMemo(() => new Map(items.map((i) => [i.id, i])), [items]);
  const grouped = useMemo(() => {
    const seen = new Set<string>();
    const out = NAV_GROUPS.map((g) => ({ title: g.title, items: g.ids.map((id) => byId.get(id)).filter((x): x is NavItem => !!x && x.enabled) }));
    out.forEach((g) => g.items.forEach((i) => seen.add(i.id)));
    const rest = items.filter((i) => i.enabled && !seen.has(i.id));
    if (rest.length) out.push({ title: "More", items: rest });
    return out.filter((g) => g.items.length);
  }, [items, byId]);
  const pick = (id: string) => { onPick(id); setQ(""); onAfterPick?.(); };
  const row = (i: { id: string; label: string }, icon?: ReactNode, sub?: string) => (
    <Box key={i.id} onClick={() => pick(i.id)} data-testid={`admin-nav-${i.id}`} role="button" tabIndex={0} onKeyDown={(e) => e.key === "Enter" && pick(i.id)}
      sx={{ display: "flex", alignItems: "center", gap: 1.25, px: 1.25, py: 0.8, borderRadius: "md", cursor: "pointer", fontSize: "sm", bgcolor: active === i.id ? "primary.softBg" : "transparent", color: active === i.id ? "primary.700" : "text.primary", fontWeight: active === i.id ? 600 : 400, "&:hover": { bgcolor: active === i.id ? "primary.softBg" : "neutral.softHoverBg" }, "& svg": { fontSize: 19 } }}>
      {icon}<Box sx={{ flex: 1, minWidth: 0 }}><Box sx={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{i.label}</Box>{sub && <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{sub}</Typography>}</Box>
    </Box>
  );
  return (
    <Box component="nav" aria-label="Admin sections" sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
      <Input size="sm" placeholder="Find a setting" value={q} onChange={(e) => setQ(e.target.value)} startDecorator={<SearchIcon fontSize="small" />} sx={{ mb: 1 }} slotProps={{ input: { "data-testid": "admin-find", "aria-label": "Find a setting" } as never }} />
      {q.trim() ? (
        hits.length ? hits.map((h) => row(h, byId.get(h.id)?.icon, h.group)) : <Typography level="body-sm" sx={{ px: 1, color: "text.tertiary" }} data-testid="admin-find-empty">No settings match “{q}”.</Typography>
      ) : grouped.map((g) => (
        <Box key={g.title} sx={{ mb: 1 }}>
          <Typography level="body-xs" sx={{ px: 1.25, py: 0.5, textTransform: "uppercase", letterSpacing: 0.6, color: "text.tertiary", fontWeight: 600 }}>{g.title}</Typography>
          {g.items.map((i) => row(i, i.icon))}
        </Box>
      ))}
    </Box>
  );
}

/** Breadcrumb trail, title and one line of help for the current page. */
export function AdminPageHeader({ id, label, onHome }: { id: string; label: string; onHome: () => void }) {
  const info = SECTION_INFO[id];
  return (
    <Box sx={{ mb: id === "dashboard" ? 2.5 : 1.5 }} data-testid="admin-page-header">
      <Breadcrumbs size="sm" sx={{ p: 0, mb: 0.5 }}>
        <Link underline="hover" color="neutral" onClick={onHome} sx={{ cursor: "pointer" }}>Admin</Link>
        {id !== "dashboard" && <Typography level="body-sm" sx={{ color: "text.tertiary" }}>{groupOf(id)}</Typography>}
        <Typography level="body-sm">{label}</Typography>
      </Breadcrumbs>
      {/* Every page except the dashboard draws its own title, so the header adds only the trail and, here, the title. */}
      {id === "dashboard" && (
        <>
          <Typography level="h3" component="h1">{label}</Typography>
          {info && <Typography level="body-sm" sx={{ color: "text.tertiary", maxWidth: 680 }}>{info.desc}</Typography>}
        </>
      )}
    </Box>
  );
}
