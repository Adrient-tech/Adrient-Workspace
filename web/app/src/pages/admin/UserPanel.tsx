import { useEffect, useState } from "react";
import { Alert, Avatar, Box, Chip, CircularProgress, Divider, Drawer, ModalClose, Sheet, Typography } from "@mui/joy";
import type { AdminUser } from "./usersApi";
import { isActive, userLabel } from "./usersApi";
import { listGroups, listMembers } from "../groups/api";
import type { Group } from "../groups/types";
import { listOrgSessions, type SessionRow } from "./orgApi";
import { listAuditEvents, type AuditEvent } from "./auditApi";
import { accessApi, type Preview } from "./accessApi";

interface Loaded {
  groups: Group[];
  sessions: SessionRow[];
  events: AuditEvent[];
  preview: Preview | null;
  notes: string[];
}

const when = (iso?: string) => (iso ? new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }) : "never");

function shortAgent(ua: string): string {
  const browser = /Edg\//.test(ua) ? "Edge" : /Chrome\//.test(ua) ? "Chrome" : /Firefox\//.test(ua) ? "Firefox" : /Safari\//.test(ua) ? "Safari" : "Browser";
  const os = /Windows/.test(ua) ? "Windows" : /Mac OS/.test(ua) ? "macOS" : /Android/.test(ua) ? "Android" : /iPhone|iPad/.test(ua) ? "iOS" : /Linux/.test(ua) ? "Linux" : "";
  return os ? `${browser} on ${os}` : browser;
}

/** Side panel with one person's profile, groups, apps, devices and recent activity. */
export function UserPanel({ user, onClose, actions }: { user: AdminUser | null; onClose: () => void; actions?: React.ReactNode }) {
  const [data, setData] = useState<Loaded | null>(null);
  useEffect(() => {
    setData(null);
    if (!user) return;
    let alive = true;
    (async () => {
      const notes: string[] = [];
      const email = user.email.toLowerCase();
      let adrientId = "";
      let groups: Group[] = [];
      try {
        const members = await listMembers();
        adrientId = members.find((m) => m.email.toLowerCase() === email)?.id ?? "";
        if (adrientId) groups = (await listGroups()).filter((g) => g.member_ids?.includes(adrientId));
        else notes.push("This person has not signed in yet, so groups and apps are not known.");
      } catch {
        notes.push("Groups could not be loaded.");
      }
      const [sessions, events, preview] = await Promise.all([
        listOrgSessions().then((s) => s.filter((x) => x.email.toLowerCase() === email && !x.revoked)).catch(() => { notes.push("Devices could not be loaded."); return [] as SessionRow[]; }),
        listAuditEvents({ actor: user.email, limit: 8 }).catch(() => { notes.push("Activity could not be loaded."); return [] as AuditEvent[]; }),
        adrientId ? accessApi.preview(adrientId).catch(() => null) : Promise.resolve(null),
      ]);
      if (alive) setData({ groups, sessions, events, preview, notes });
    })();
    return () => { alive = false; };
  }, [user]);

  return (
    <Drawer open={!!user} onClose={onClose} anchor="right" size="md" slotProps={{ content: { sx: { width: { xs: "100%", sm: 440 }, p: 0 } } }}>
      {user && (
        <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }} data-testid="user-panel">
          <Box sx={{ p: 2.5, pb: 1.5, display: "flex", gap: 1.5, alignItems: "center" }}>
            <Avatar size="lg">{userLabel(user).charAt(0).toUpperCase()}</Avatar>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography level="title-lg" noWrap>{userLabel(user)}</Typography>
              <Typography level="body-sm" sx={{ color: "text.tertiary" }} noWrap>{user.email}</Typography>
              <Box sx={{ display: "flex", gap: 0.75, mt: 0.5 }}>
                <Chip size="sm" variant="soft" color={isActive(user.state) ? "success" : "neutral"}>{isActive(user.state) ? "Active" : "Suspended"}</Chip>
                {user.isAdmin && <Chip size="sm" variant="soft" color="primary">Admin</Chip>}
                {!user.emailVerified && <Chip size="sm" variant="soft" color="warning">Email not verified</Chip>}
              </Box>
            </Box>
            <ModalClose sx={{ position: "static" }} />
          </Box>
          {actions && <Box sx={{ px: 2.5, pb: 1.5, display: "flex", gap: 1, flexWrap: "wrap" }}>{actions}</Box>}
          <Divider />
          <Box sx={{ flex: 1, overflow: "auto", p: 2.5, display: "flex", flexDirection: "column", gap: 2.5 }}>
            {!data ? <Box sx={{ display: "grid", placeItems: "center", py: 6 }}><CircularProgress /></Box> : (
              <>
                {data.notes.map((n) => <Alert key={n} size="sm" color="neutral">{n}</Alert>)}
                <Section title="Profile">
                  <Row k="Username" v={user.username || "-"} />
                  <Row k="Given name" v={user.givenName || "-"} />
                  <Row k="Family name" v={user.familyName || "-"} />
                </Section>
                <Section title={`Groups (${data.groups.length})`}>
                  {data.groups.length === 0 ? <Typography level="body-sm" sx={{ color: "text.tertiary" }}>Not in any group.</Typography> : (
                    <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap" }}>{data.groups.map((g) => <Chip key={g.id} size="sm" variant="outlined">{g.name}</Chip>)}</Box>
                  )}
                </Section>
                <Section title="Apps">
                  {!data.preview ? <Typography level="body-sm" sx={{ color: "text.tertiary" }}>Not available.</Typography> : (
                    <>
                      <Typography level="body-xs" sx={{ color: "text.tertiary", mb: 0.5 }}>{data.preview.allowed.length} allowed, {data.preview.denied.length} blocked</Typography>
                      <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                        {data.preview.allowed.slice(0, 40).map((a) => <Chip key={a} size="sm" variant="soft" color="success">{a}</Chip>)}
                        {data.preview.denied.slice(0, 20).map((a) => <Chip key={a} size="sm" variant="soft" color="danger">{a}</Chip>)}
                      </Box>
                    </>
                  )}
                </Section>
                <Section title={`Devices (${data.sessions.length})`}>
                  {data.sessions.length === 0 ? <Typography level="body-sm" sx={{ color: "text.tertiary" }}>No active sign-ins.</Typography> : data.sessions.slice(0, 8).map((s) => (
                    <Sheet key={s.id} variant="soft" sx={{ p: 1, borderRadius: "md", mb: 0.75 }}>
                      <Typography level="body-sm">{shortAgent(s.user_agent)}{s.current ? " (this device)" : ""}</Typography>
                      <Typography level="body-xs" sx={{ color: "text.tertiary" }}>{s.ip || "unknown address"} · last seen {when(s.last_seen_at ?? s.created_at)}</Typography>
                    </Sheet>
                  ))}
                </Section>
                <Section title="Recent activity">
                  {data.events.length === 0 ? <Typography level="body-sm" sx={{ color: "text.tertiary" }}>No activity recorded.</Typography> : data.events.map((e) => (
                    <Box key={e.id} sx={{ display: "flex", gap: 1, py: 0.4 }}>
                      <Typography level="body-sm" sx={{ flex: 1 }} noWrap>{e.service}: {e.action}</Typography>
                      <Typography level="body-xs" sx={{ color: e.status === "ok" ? "text.tertiary" : "danger.600" }}>{e.status !== "ok" ? `${e.status} · ` : ""}{when(e.created_at)}</Typography>
                    </Box>
                  ))}
                </Section>
              </>
            )}
          </Box>
        </Box>
      )}
    </Drawer>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Box>
      <Typography level="title-sm" sx={{ mb: 0.75 }}>{title}</Typography>
      {children}
    </Box>
  );
}
function Row({ k, v }: { k: string; v: string }) {
  return (
    <Box sx={{ display: "flex", gap: 1, py: 0.25 }}>
      <Typography level="body-sm" sx={{ width: 110, color: "text.tertiary" }}>{k}</Typography>
      <Typography level="body-sm" sx={{ flex: 1, minWidth: 0 }} noWrap>{v}</Typography>
    </Box>
  );
}
