// User types and lifecycle: filter by type and status, invite guests, external
// members and members, block and unblock, change type and expiry, and manage
// pending invitations. Invitations return a copyable link, and an e-mail is
// sent when the mail sender is configured.
import { LoadOlder } from "../../components/LoadOlder";
import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  DialogContent,
  DialogTitle,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Table,
  Typography,
} from "@mui/joy";
import {
  identityApi,
  type IdentityUser,
  type Invitation,
  type InviteResult,
  type UserType,
} from "./identityApi";

const TYPE_LABEL: Record<UserType, string> = {
  member: "Member",
  external: "External",
  guest: "Guest",
  service: "Service",
};
const TYPE_COLOR = { member: "neutral", external: "warning", guest: "primary", service: "neutral" } as const;
const GUEST_APPS = ["drive", "docs", "sheets", "slides", "meet", "chat"];

const fmt = (s: string | null | undefined) => (s ? new Date(s).toLocaleDateString() : "");

export function IdentityUsersSection() {
  const [users, setUsers] = useState<IdentityUser[]>([]);
  const [invites, setInvites] = useState<Invitation[]>([]);
  const [type, setType] = useState("");
  const [status, setStatus] = useState("");
  const [q, setQ] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [nextUsers, setNextUsers] = useState("");
  const [link, setLink] = useState<InviteResult | null>(null);

  const load = useCallback(async () => {
    try {
      const [u, i] = await Promise.all([identityApi.users({ type, status, q }), identityApi.invitations()]);
      setUsers(u.users);
      setNextUsers(u.next_cursor ?? "");
      setInvites(i.invitations);
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    }
  }, [type, status, q]);
  useEffect(() => {
    const t = setTimeout(() => void load(), 200);
    return () => clearTimeout(t);
  }, [load]);

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    setError(null);
    setNotice(null);
    try {
      await fn();
      setNotice(ok);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <Box data-testid="identity-users">
      <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
        <Typography level="h3" sx={{ flex: 1 }}>
          User types and invitations
        </Typography>
        <Button variant="outlined" size="sm" sx={{ mr: 1 }} onClick={() => act(async () => {
          const r = await identityApi.runExpiry();
          setNotice(`Expiry run: ${r.blocked} account(s) blocked.`);
        }, "Expiry job finished.")}>
          Run expiry now
        </Button>
        <Button onClick={() => setInviteOpen(true)}>Invite</Button>
      </Box>
      {error && (
        <Alert color="danger" sx={{ mb: 1.5 }}>
          {error}
        </Alert>
      )}
      {notice && (
        <Alert color="success" sx={{ mb: 1.5 }}>
          {notice}
        </Alert>
      )}

      <Box sx={{ display: "flex", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
        <Input placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)} sx={{ width: 220 }}
          slotProps={{ input: { "aria-label": "Search users" } }} />
        <Select value={type} onChange={(_, v) => setType(v ?? "")} sx={{ width: 160 }} slotProps={{ button: { "aria-label": "Type filter" } }}>
          <Option value="">All types</Option>
          <Option value="member">Member</Option>
          <Option value="external">External</Option>
          <Option value="guest">Guest</Option>
          <Option value="service">Service</Option>
        </Select>
        <Select value={status} onChange={(_, v) => setStatus(v ?? "")} sx={{ width: 160 }} slotProps={{ button: { "aria-label": "Status filter" } }}>
          <Option value="">All statuses</Option>
          <Option value="active">Active</Option>
          <Option value="blocked">Blocked</Option>
        </Select>
      </Box>

      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", mb: 3 }}>
        <Table stickyHeader hoverRow sx={{ tableLayout: "auto", "& td, & th": { whiteSpace: "nowrap" } }}>
          <thead>
            <tr>
              <th>User</th>
              <th style={{ width: 100 }}>Type</th>
              <th style={{ width: 110 }}>Status</th>
              <th style={{ width: 110 }}>Expires</th>
              <th>Sponsor</th>
              <th style={{ width: 110 }}>Last sign-in</th>
              <th style={{ width: 200 }} />
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} data-testid={`urow-${u.email}`}>
                <td>
                  <Typography level="title-sm">{u.display_name || u.email}</Typography>
                  <Typography level="body-xs">{u.upn || u.email}</Typography>
                </td>
                <td>
                  <Chip size="sm" variant="soft" color={TYPE_COLOR[u.user_type]}>
                    {TYPE_LABEL[u.user_type]}
                  </Chip>
                </td>
                <td>
                  <Chip size="sm" variant="soft" color={u.status === "active" ? "success" : "danger"}>
                    {u.status === "active" ? "active" : u.blocked_reason === "expired" ? "expired" : "blocked"}
                  </Chip>
                </td>
                <td>{fmt(u.expires_at)}</td>
                <td>{u.sponsor_email}</td>
                <td>{fmt(u.last_sign_in_at)}</td>
                <td>
                  {u.status === "active" ? (
                    <Button size="sm" color="danger" variant="plain" onClick={() => act(() => identityApi.block(u.id, "blocked by admin"), `${u.email} blocked.`)}>
                      Block
                    </Button>
                  ) : (
                    <Button size="sm" variant="plain" onClick={() => act(() => identityApi.unblock(u.id), `${u.email} unblocked.`)}>
                      Unblock
                    </Button>
                  )}
                  {(u.user_type === "guest" || u.user_type === "external") && (
                    <Button size="sm" variant="plain" onClick={() => act(() => identityApi.patchUser(u.id, { expires_at: new Date(Date.now() + 30 * 864e5).toISOString() }), `Extended ${u.email} by 30 days.`)}>
                      +30d
                    </Button>
                  )}
                </td>
              </tr>
            ))}
            {users.length === 0 && (
              <tr>
                <td colSpan={7}>
                  <Typography level="body-sm">No users match.</Typography>
                </td>
              </tr>
            )}
          </tbody>
        </Table>
        <LoadOlder next={nextUsers} shown={users?.length ?? 0} label="Load more people" testId="ident-more" loadMore={async (c) => {
          const r = await identityApi.users({ type, status, q, page: Number(c) });
          setUsers((x) => [...(x ?? []), ...r.users]);
          setNextUsers(r.next_cursor ?? "");
        }} />
      </Sheet>

      <Typography level="title-md" sx={{ mb: 1 }}>
        Invitations
      </Typography>
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
        <Table sx={{ tableLayout: "auto" }}>
          <thead>
            <tr>
              <th>Email</th>
              <th style={{ width: 100 }}>Type</th>
              <th style={{ width: 110 }}>State</th>
              <th style={{ width: 120 }}>Valid until</th>
              <th style={{ width: 180 }} />
            </tr>
          </thead>
          <tbody>
            {invites.map((i) => (
              <tr key={i.id} data-testid={`irow-${i.email}`}>
                <td>{i.email}</td>
                <td>{TYPE_LABEL[i.user_type]}</td>
                <td>
                  <Chip size="sm" variant="soft" color={i.state === "pending" ? "warning" : i.state === "accepted" ? "success" : "neutral"}>
                    {i.state}
                  </Chip>
                </td>
                <td>{fmt(i.expires_at)}</td>
                <td>
                  {i.state === "pending" && (
                    <>
                      <Button size="sm" variant="plain" onClick={async () => { try { setLink(await identityApi.resend(i.id)); await load(); } catch (e) { setError((e as Error).message); } }}>
                        Resend
                      </Button>
                      <Button size="sm" color="danger" variant="plain" onClick={() => act(() => identityApi.revoke(i.id), "Invitation revoked.")}>
                        Revoke
                      </Button>
                    </>
                  )}
                </td>
              </tr>
            ))}
            {invites.length === 0 && (
              <tr>
                <td colSpan={5}>
                  <Typography level="body-sm">No invitations yet.</Typography>
                </td>
              </tr>
            )}
          </tbody>
        </Table>
      </Sheet>

      <InviteDialog
        open={inviteOpen}
        onClose={() => setInviteOpen(false)}
        onDone={(r) => {
          setInviteOpen(false);
          setLink(r);
          void load();
        }}
      />
      <Modal open={!!link} onClose={() => setLink(null)}>
        <ModalDialog sx={{ width: 520 }}>
          <DialogTitle>Invitation link</DialogTitle>
          <DialogContent>
            <Typography level="body-sm" sx={{ mb: 1 }}>
              {link?.email_sent ? "An e-mail was sent. " : "E-mail is not configured, so share this link yourself. "}
              It works once and expires in 7 days.
            </Typography>
            <Input readOnly value={link?.accept_url ?? ""} slotProps={{ input: { "aria-label": "Invitation link", "data-testid": "invite-link" } }} />
            <Box sx={{ mt: 2, display: "flex", gap: 1, justifyContent: "flex-end" }}>
              <Button variant="outlined" onClick={() => void navigator.clipboard?.writeText(link?.accept_url ?? "")}>
                Copy link
              </Button>
              <Button onClick={() => setLink(null)}>Done</Button>
            </Box>
          </DialogContent>
        </ModalDialog>
      </Modal>
    </Box>
  );
}

function InviteDialog({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: (r: InviteResult) => void }) {
  const [email, setEmail] = useState("");
  const [type, setType] = useState<UserType>("guest");
  const [days, setDays] = useState("");
  const [apps, setApps] = useState<string[]>(["drive"]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const r = await identityApi.invite({
        email: email.trim(),
        user_type: type,
        expire_days: days ? Number(days) : undefined,
        app_ids: type === "guest" ? apps : undefined,
      });
      setEmail("");
      onDone(r);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 460 }} data-testid="invite-dialog">
        <DialogTitle>Invite someone</DialogTitle>
        <DialogContent>
          {err && (
            <Alert color="danger" sx={{ mb: 1 }}>
              {err}
            </Alert>
          )}
          <FormControl sx={{ mb: 1.5 }}>
            <FormLabel>Email</FormLabel>
            <Input value={email} onChange={(e) => setEmail(e.target.value)} type="email" autoFocus slotProps={{ input: { "aria-label": "Invite email" } }} />
          </FormControl>
          <FormControl sx={{ mb: 1.5 }}>
            <FormLabel>Type</FormLabel>
            <Select value={type} onChange={(_, v) => v && setType(v)} slotProps={{ button: { "aria-label": "Invite type" } }}>
              <Option value="guest">Guest (30 days, limited apps)</Option>
              <Option value="external">External member / contractor (90 days)</Option>
              <Option value="member">Member (no expiry)</Option>
            </Select>
          </FormControl>
          {type !== "member" && (
            <FormControl sx={{ mb: 1.5 }}>
              <FormLabel>Expires after (days)</FormLabel>
              <Input value={days} onChange={(e) => setDays(e.target.value.replace(/\D/g, ""))} placeholder={type === "guest" ? "30" : "90"} />
            </FormControl>
          )}
          {type === "guest" && (
            <FormControl sx={{ mb: 1.5 }}>
              <FormLabel>Apps the guest can open</FormLabel>
              <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
                {GUEST_APPS.map((a) => (
                  <Checkbox key={a} size="sm" label={a} checked={apps.includes(a)}
                    onChange={(e) => setApps(e.target.checked ? [...apps, a] : apps.filter((x) => x !== a))} />
                ))}
              </Box>
            </FormControl>
          )}
          <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end", mt: 1 }}>
            <Button variant="plain" onClick={onClose}>
              Cancel
            </Button>
            <Button loading={busy} disabled={!email.includes("@")} onClick={submit}>
              Send invitation
            </Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}
