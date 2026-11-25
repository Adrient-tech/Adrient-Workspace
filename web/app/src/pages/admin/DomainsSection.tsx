// Domains: claim a domain, publish the TXT record, verify it, pick the primary
// domain and decide who may sign up or auto-join. Org sign-up mode and the
// allowed sign-in domains live at the bottom.
import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  Input,
  Option,
  Select,
  Sheet,
  Table,
  Typography,
} from "@mui/joy";
import {
  identityApi,
  type OrgDomain,
  type OrgSettings,
  type SignupMode,
} from "./identityApi";
import { DomainSetupWizard } from "./DomainSetupWizard";

const STATUS_COLOR = { verified: "success", pending: "warning", failed: "danger", removed: "neutral" } as const;

const MODE_LABEL: Record<SignupMode, string> = {
  invite_only: "Invite only (default)",
  domain_auto_join: "Auto-join with a verified domain",
  domain_approval: "Verified domain, admin approval",
  open: "Open sign-up (not recommended)",
};

export function DomainsSection() {
  const [domains, setDomains] = useState<OrgDomain[]>([]);
  const [settings, setSettings] = useState<OrgSettings | null>(null);
  const [newDomain, setNewDomain] = useState("");
  const [allowed, setAllowed] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [setupOpen, setSetupOpen] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [d, s] = await Promise.all([identityApi.domains(), identityApi.settings()]);
      setDomains(d.domains);
      setSettings(s);
      setAllowed(s.allowed_signin_domains.join(", "));
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const run = async (key: string, fn: () => Promise<unknown>, ok?: string) => {
    setBusy(key);
    setError(null);
    setNotice(null);
    try {
      await fn();
      if (ok) setNotice(ok);
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const saveSettings = () =>
    run(
      "settings",
      () =>
        identityApi.putSettings({
          ...settings!,
          allowed_signin_domains: allowed
            .split(/[\s,]+/)
            .map((s) => s.trim())
            .filter(Boolean),
        }),
      "Sign-up settings saved.",
    );

  return (
    <Box data-testid="domains-section">
      <Typography level="h3" sx={{ mb: 0.5 }}>
        Domains
      </Typography>
      <Typography level="body-sm" sx={{ mb: 2 }}>
        Prove you own a domain by publishing a TXT record. Verified domains give addresses, the primary
        sign-in suffix and optional auto-join.
      </Typography>
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

      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2, display: "flex", gap: 1 }}>
        <Input
          placeholder="example.com"
          value={newDomain}
          onChange={(e) => setNewDomain(e.target.value)}
          sx={{ flex: 1 }}
          slotProps={{ input: { "aria-label": "New domain" } }}
        />
        <Button
          loading={busy === "add"}
          disabled={!newDomain.trim()}
          onClick={() =>
            run("add", async () => {
              await identityApi.addDomain(newDomain.trim());
              setNewDomain("");
            })
          }
        >
          Add domain
        </Button>
      </Sheet>

      {domains.map((d) => (
        <Sheet key={d.id} variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 1.5 }} data-testid={`domain-${d.domain}`}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
            <Typography level="title-md">{d.domain}</Typography>
            <Chip size="sm" color={STATUS_COLOR[d.status]} variant="soft">
              {d.status}
            </Chip>
            {d.is_primary && (
              <Chip size="sm" color="primary" variant="solid">
                primary
              </Chip>
            )}
            <Typography level="body-xs" sx={{ ml: "auto" }}>
              {d.user_count} user{d.user_count === 1 ? "" : "s"}
            </Typography>
          </Box>

          {d.status !== "verified" && (
            <Box sx={{ mt: 1.5 }}>
              <Typography level="body-sm">
                Step 2. Create this TXT record at your DNS provider, then press Verify.
              </Typography>
              <Table size="sm" sx={{ my: 1 }}>
                <tbody>
                  <tr>
                    <td style={{ width: 80 }}>Type</td>
                    <td>TXT</td>
                  </tr>
                  <tr>
                    <td>Name</td>
                    <td>
                      <code>{d.verify_host}</code>
                    </td>
                  </tr>
                  <tr>
                    <td>Value</td>
                    <td>
                      <code data-testid="txt-value">{d.verify_txt}</code>
                    </td>
                  </tr>
                </tbody>
              </Table>
              <Button size="sm" loading={busy === `v${d.id}`} onClick={() => run(`v${d.id}`, async () => {
                const r = await identityApi.verifyDomain(d.id);
                if (!r.verified) throw new Error("TXT record not found yet. DNS can take a few minutes.");
              }, `${d.domain} verified.`)}>
                Verify
              </Button>
            </Box>
          )}

          {d.status === "verified" && (
            <Box sx={{ mt: 1.5, display: "flex", gap: 2, flexWrap: "wrap", alignItems: "center" }}>
              <Checkbox
                size="sm"
                label="Primary"
                checked={d.is_primary}
                disabled={d.is_primary}
                onChange={() => run(`p${d.id}`, () => identityApi.patchDomain(d.id, { is_primary: true }))}
              />
              <Checkbox
                size="sm"
                label="Allow sign-up"
                checked={d.allow_signup}
                onChange={(e) => run(`s${d.id}`, () => identityApi.patchDomain(d.id, { allow_signup: e.target.checked }))}
              />
              <Checkbox
                size="sm"
                label="Auto-join"
                checked={d.auto_join}
                onChange={(e) => run(`a${d.id}`, () => identityApi.patchDomain(d.id, { auto_join: e.target.checked }))}
              />
              <Button
                size="sm"
                variant="outlined"
                onClick={() => setSetupOpen(setupOpen === d.id ? null : d.id)}
              >
                {setupOpen === d.id ? "Hide domain setup" : "Domain setup"}
              </Button>
            </Box>
          )}

          {d.status === "verified" && setupOpen === d.id && <DomainSetupWizard domainId={d.id} domain={d.domain} />}

          {!d.is_primary && (
            <Button
              size="sm"
              color="danger"
              variant="plain"
              sx={{ mt: 1 }}
              onClick={() => run(`r${d.id}`, () => identityApi.removeDomain(d.id), `${d.domain} removed.`)}
            >
              Remove
            </Button>
          )}
        </Sheet>
      ))}
      {domains.length === 0 && (
        <Typography level="body-sm" sx={{ mb: 2 }}>
          No domains yet. Add one above.
        </Typography>
      )}

      {settings && (
        <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mt: 3 }}>
          <Typography level="title-md" sx={{ mb: 1 }}>
            Sign-up and sign-in
          </Typography>
          <Typography level="body-sm" sx={{ mb: 0.5 }}>
            Sign-up mode
          </Typography>
          <Select
            value={settings.signup_mode}
            onChange={(_, v) => v && setSettings({ ...settings, signup_mode: v })}
            sx={{ maxWidth: 380, mb: 2 }}
            slotProps={{ button: { "aria-label": "Sign-up mode" } }}
          >
            {(Object.keys(MODE_LABEL) as SignupMode[]).map((m) => (
              <Option key={m} value={m}>
                {MODE_LABEL[m]}
              </Option>
            ))}
          </Select>
          <Typography level="body-sm" sx={{ mb: 0.5 }}>
            Allowed sign-in domains (members only; empty = any). Guests and admins are not affected.
          </Typography>
          <Input
            value={allowed}
            onChange={(e) => setAllowed(e.target.value)}
            placeholder="example.com, example.org"
            sx={{ maxWidth: 480, mb: 2 }}
            slotProps={{ input: { "aria-label": "Allowed sign-in domains" } }}
          />
          <Box>
            <Button loading={busy === "settings"} onClick={saveSettings}>
              Save
            </Button>
          </Box>
        </Sheet>
      )}
    </Box>
  );
}
