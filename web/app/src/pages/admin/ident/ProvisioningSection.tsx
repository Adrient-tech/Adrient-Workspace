// Single sign-on (Entra ID, Google, OIDC, SAML through Zitadel) and SCIM
// provisioning tokens, in one place.
import { LoadOlder } from "../../../components/LoadOlder";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, FormControl, FormHelperText, FormLabel, Input, Option, Select, Sheet, Switch, Table, Textarea, Typography } from "@mui/joy";
import { Confirm, Notice, Page, api, when } from "./common";

interface Conn {
  id: string;
  kind: string;
  name: string;
  domains: string[];
  jit: boolean;
  enforce: boolean;
  enabled: boolean;
}
interface ScimToken {
  id: string;
  name: string;
  prefix: string;
  rate_per_min: number;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
  revoked_at: string | null;
}
interface ScimEvent {
  id: number;
  method: string;
  path: string;
  status: number;
  created_at: string;
}

const KINDS: Record<string, string> = { entra: "Microsoft Entra ID", google: "Google", oidc: "OpenID Connect", saml: "SAML 2.0" };

export function ProvisioningSection() {
  const [conns, setConns] = useState<Conn[]>([]);
  const [domains, setDomains] = useState<string[]>([]);
  const [connected, setConnected] = useState(true);
  const [tokens, setTokens] = useState<ScimToken[]>([]);
  const [events, setEvents] = useState<ScimEvent[]>([]);
  const [nextEvents, setNextEvents] = useState("");
  const [showAll, setShowAll] = useState(false);
  const [baseUrl, setBaseUrl] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [newToken, setNewToken] = useState<string | null>(null);
  const [tokenName, setTokenName] = useState("");
  const [rm, setRm] = useState<Conn | null>(null);

  const [kind, setKind] = useState("entra");
  const [name, setName] = useState("");
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [tenant, setTenant] = useState("");
  const [issuer, setIssuer] = useState("");
  const [meta, setMeta] = useState("");
  const [doms, setDoms] = useState("");
  const [jit, setJit] = useState(true);
  const [enforce, setEnforce] = useState(false);

  const load = useCallback(async () => {
    try {
      const s = await api<{ connections: Conn[]; verified_domains: string[]; zitadel_connected: boolean }>("/api/v1/admin/sso");
      setConns(s.connections);
      setDomains(s.verified_domains ?? []);
      setConnected(s.zitadel_connected);
      const [t, e, i] = await Promise.all([
        api<{ tokens: ScimToken[] }>("/api/v1/admin/scim/tokens"),
        api<{ events: ScimEvent[]; next_cursor?: string }>("/api/v1/admin/scim/events"),
        api<{ base_url: string }>("/api/v1/admin/scim/info"),
      ]);
      setTokens(t.tokens);
      setEvents(e.events);
      setNextEvents(e.next_cursor ?? "");
      setBaseUrl(i.base_url);
    } catch (err) {
      setError((err as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function addConn() {
    setError(null);
    setNotice(null);
    try {
      await api("/api/v1/admin/sso", {
        method: "POST",
        json: {
          kind,
          name,
          client_id: clientId,
          client_secret: secret,
          tenant_id: tenant,
          issuer,
          metadata_xml: kind === "saml" && meta.trim().startsWith("<") ? meta : "",
          metadata_url: kind === "saml" && meta.trim().startsWith("http") ? meta.trim() : "",
          domains: doms.split(/[\s,]+/).filter(Boolean),
          jit,
          enforce,
        },
      });
      setNotice(`${KINDS[kind]} connection added in Zitadel. People with those e-mail domains now go straight to it.`);
      setName("");
      setClientId("");
      setSecret("");
      setDoms("");
      setMeta("");
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  }
  async function patch(c: Conn, body: Partial<Conn>) {
    setError(null);
    try {
      await api(`/api/v1/admin/sso/${c.id}`, { method: "PATCH", json: body });
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  }
  async function makeToken() {
    setError(null);
    try {
      const r = await api<{ token: string }>("/api/v1/admin/scim/tokens", { method: "POST", json: { name: tokenName } });
      setNewToken(r.token);
      setTokenName("");
      await load();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  return (
    <Page
      testId="provisioning"
      title="Sign-in and provisioning"
      intro="Let people sign in with Microsoft Entra ID or Google by e-mail domain, and let Entra, Okta or Google create, update and remove accounts automatically with SCIM."
    >
      <Notice error={error} notice={notice} />
      {!connected && (
        <Sheet variant="soft" color="warning" sx={{ p: 1.5, mb: 2, borderRadius: "md" }}>
          Zitadel is not connected to this server, so providers cannot be created here. Set ADRIENT_ZITADEL_API_URL and
          ADRIENT_ZITADEL_SERVICE_TOKEN, or follow docs/sso-scim.md.
        </Sheet>
      )}

      <Typography level="title-md" sx={{ mb: 1 }}>
        Single sign-on
      </Typography>
      <Table size="sm" sx={{ mb: 2, maxWidth: 900 }} data-testid="sso-table">
        <thead>
          <tr>
            <th>Provider</th>
            <th>Domains</th>
            <th>Create accounts at first sign-in</th>
            <th>Require SSO</th>
            <th>On</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {conns.length === 0 && (
            <tr>
              <td colSpan={6}>No provider yet.</td>
            </tr>
          )}
          {conns.map((c) => (
            <tr key={c.id} data-testid={`sso-row-${c.name}`}>
              <td>
                {c.name} <Chip size="sm">{KINDS[c.kind] ?? c.kind}</Chip>
              </td>
              <td>{c.domains.join(", ")}</td>
              <td>
                <Switch checked={c.jit} onChange={(e) => void patch(c, { jit: e.target.checked })} />
              </td>
              <td>
                <Switch checked={c.enforce} onChange={(e) => void patch(c, { enforce: e.target.checked })} />
              </td>
              <td>
                <Switch checked={c.enabled} onChange={(e) => void patch(c, { enabled: e.target.checked })} />
              </td>
              <td>
                <Button size="sm" variant="plain" color="danger" onClick={() => setRm(c)}>
                  Remove
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </Table>

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 3, maxWidth: 760, display: "grid", gap: 1.25 }}>
        <Typography level="title-sm">Add a provider</Typography>
        <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
          <FormControl size="sm">
            <FormLabel>Type</FormLabel>
            <Select size="sm" value={kind} onChange={(_, v) => v && setKind(v)} sx={{ width: 200 }} data-testid="sso-kind">
              {Object.entries(KINDS).map(([k, l]) => (
                <Option key={k} value={k}>
                  {l}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl size="sm" sx={{ flex: 1, minWidth: 200 }}>
            <FormLabel>Name</FormLabel>
            <Input size="sm" value={name} onChange={(e) => setName(e.target.value)} placeholder="Contoso Entra" slotProps={{ input: { "data-testid": "sso-name" } as never }} />
          </FormControl>
        </Box>
        {kind !== "saml" && (
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
            <FormControl size="sm" sx={{ flex: 1, minWidth: 200 }}>
              <FormLabel>Client ID</FormLabel>
              <Input size="sm" value={clientId} onChange={(e) => setClientId(e.target.value)} slotProps={{ input: { "data-testid": "sso-client-id" } as never }} />
            </FormControl>
            <FormControl size="sm" sx={{ flex: 1, minWidth: 200 }}>
              <FormLabel>Client secret</FormLabel>
              <Input size="sm" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} slotProps={{ input: { "data-testid": "sso-secret", autoComplete: "new-password" } as never }} />
              <FormHelperText>Sent to Zitadel and not stored here.</FormHelperText>
            </FormControl>
          </Box>
        )}
        {kind === "entra" && (
          <FormControl size="sm">
            <FormLabel>Directory (tenant) ID</FormLabel>
            <Input size="sm" value={tenant} onChange={(e) => setTenant(e.target.value)} placeholder="a GUID, or leave empty for any Microsoft account" />
          </FormControl>
        )}
        {kind === "oidc" && (
          <FormControl size="sm">
            <FormLabel>Issuer URL</FormLabel>
            <Input size="sm" value={issuer} onChange={(e) => setIssuer(e.target.value)} placeholder="https://login.example.com" slotProps={{ input: { "data-testid": "sso-issuer" } as never }} />
          </FormControl>
        )}
        {kind === "saml" && (
          <FormControl size="sm">
            <FormLabel>Identity provider metadata (XML, or a metadata URL)</FormLabel>
            <Textarea minRows={3} value={meta} onChange={(e) => setMeta(e.target.value)} />
          </FormControl>
        )}
        <FormControl size="sm">
          <FormLabel>E-mail domains that use this provider</FormLabel>
          <Input size="sm" value={doms} onChange={(e) => setDoms(e.target.value)} placeholder={domains[0] ?? "example.com"} slotProps={{ input: { "data-testid": "sso-domains" } as never }} />
          <FormHelperText>Verified domains only: {domains.join(", ") || "none yet, add one under Domains"}.</FormHelperText>
        </FormControl>
        <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap" }}>
          <FormControl size="sm" orientation="horizontal" sx={{ gap: 1 }}>
            <Switch checked={jit} onChange={(e) => setJit(e.target.checked)} />
            <FormLabel>Create accounts at first sign-in</FormLabel>
          </FormControl>
          <FormControl size="sm" orientation="horizontal" sx={{ gap: 1 }}>
            <Switch checked={enforce} onChange={(e) => setEnforce(e.target.checked)} />
            <FormLabel>Require SSO (admins keep a password as break-glass)</FormLabel>
          </FormControl>
        </Box>
        <Box>
          <Button size="sm" onClick={addConn} disabled={!name.trim() || !doms.trim()} data-testid="sso-add">
            Add provider
          </Button>
        </Box>
        <Typography level="body-xs" sx={{ opacity: 0.7 }}>
          In Entra or Google, register the redirect URI shown in Zitadel for this provider (Zitadel &gt; Identity providers). This page creates the provider
          in Zitadel for you.
        </Typography>
      </Sheet>

      <Typography level="title-md" sx={{ mb: 0.5 }}>
        SCIM provisioning
      </Typography>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        Base URL for Entra, Okta and Google: <code data-testid="scim-base">{baseUrl}</code>. Authentication is a bearer token.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 1.5, maxWidth: 520 }}>
        <Input size="sm" sx={{ flex: 1 }} placeholder="Token name, for example Entra production" value={tokenName} onChange={(e) => setTokenName(e.target.value)} slotProps={{ input: { "data-testid": "scim-token-name" } as never }} />
        <Button size="sm" disabled={!tokenName.trim()} onClick={makeToken} data-testid="scim-token-add">
          Create token
        </Button>
      </Box>
      {newToken && (
        <Sheet variant="soft" color="success" sx={{ p: 1.5, mb: 1.5, borderRadius: "md", maxWidth: 760 }} data-testid="scim-token-once">
          Copy this token now, it is not shown again:
          <Box component="code" sx={{ display: "block", wordBreak: "break-all", mt: 0.5 }}>
            {newToken}
          </Box>
        </Sheet>
      )}
      <Table size="sm" sx={{ maxWidth: 900, mb: 2 }} data-testid="scim-tokens">
        <thead>
          <tr>
            <th>Name</th>
            <th>Token</th>
            <th>Limit</th>
            <th>Last used</th>
            <th>Status</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {tokens.map((t) => (
            <tr key={t.id}>
              <td>{t.name}</td>
              <td>
                <code>{t.prefix}…</code>
              </td>
              <td>{t.rate_per_min}/min</td>
              <td>{t.last_used_at ? when(t.last_used_at) : "never"}</td>
              <td>{t.revoked_at ? "revoked" : "active"}</td>
              <td>
                {!t.revoked_at && (
                  <Button size="sm" variant="plain" color="danger" onClick={() => void api(`/api/v1/admin/scim/tokens/${t.id}`, { method: "DELETE" }).then(load)}>
                    Revoke
                  </Button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
      <Typography level="title-sm" sx={{ mb: 0.5 }}>
        Recent provisioning requests
      </Typography>
      <Table size="sm" sx={{ maxWidth: 900 }} data-testid="scim-events">
        <tbody>
          {(showAll ? events : events.slice(0, 12)).map((e) => (
            <tr key={e.id}>
              <td>{when(e.created_at)}</td>
              <td>{e.method}</td>
              <td>{e.path}</td>
              <td>{e.status}</td>
            </tr>
          ))}
          {events.length === 0 && (
            <tr>
              <td>No requests yet.</td>
            </tr>
          )}
        </tbody>
      </Table>
      {!showAll && events.length > 12 ? (
        <Button size="sm" variant="plain" onClick={() => setShowAll(true)} data-testid="scim-events-all">
          Show all {events.length} loaded requests
        </Button>
      ) : (
        <LoadOlder
          next={showAll ? nextEvents : ""}
          shown={showAll ? events.length : 0}
          testId="scim-older"
          loadMore={async (c) => {
            const r = await api<{ events: ScimEvent[]; next_cursor?: string }>(`/api/v1/admin/scim/events?cursor=${encodeURIComponent(c)}`);
            setEvents((x) => [...x, ...r.events]);
            setNextEvents(r.next_cursor ?? "");
          }}
        />
      )}

      <Confirm
        open={!!rm}
        title="Remove this provider?"
        danger
        confirmLabel="Remove"
        body={<>People on {rm?.domains.join(", ")} will sign in with a password again. The provider is deleted from Zitadel.</>}
        onCancel={() => setRm(null)}
        onConfirm={async () => {
          if (!rm) return;
          try {
            await api(`/api/v1/admin/sso/${rm.id}`, { method: "DELETE" });
            setRm(null);
            await load();
          } catch (err) {
            setError((err as Error).message);
            setRm(null);
          }
        }}
      />
    </Page>
  );
}
