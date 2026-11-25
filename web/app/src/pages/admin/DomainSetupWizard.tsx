// "Domain setup" wizard: the mail DNS records for one verified domain, with
// copy buttons, live pass/warn/fail chips, expected vs found, DKIM key control
// and the DMARC ramp hint. Re-checks every 15 s while open.
import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Box, Button, Chip, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { dnsApi, type CheckStatus, type DnsPlan, type DnsResult, type MailSettings } from "./dnsApi";

const COLOR = { pass: "success", warn: "warning", fail: "danger", unknown: "neutral" } as const;
const LABEL = { pass: "OK", warn: "Check", fail: "Missing", unknown: "Not checked" } as const;

export function CopyButton({ text, label }: { text: string; label: string }) {
  const [done, setDone] = useState(false);
  return (
    <Button
      size="sm"
      variant="plain"
      aria-label={label}
      onClick={() => {
        void navigator.clipboard?.writeText(text);
        setDone(true);
        setTimeout(() => setDone(false), 1500);
      }}
    >
      {done ? "Copied" : "Copy"}
    </Button>
  );
}

export function DomainSetupWizard({ domainId, domain, pollMs = 15000 }: { domainId: string; domain: string; pollMs?: number }) {
  const [plan, setPlan] = useState<DnsPlan | null>(null);
  const [results, setResults] = useState<Record<string, DnsResult>>({});
  const [hint, setHint] = useState("");
  const [days, setDays] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [checkedAt, setCheckedAt] = useState<Date | null>(null);
  const alive = useRef(true);

  const loadPlan = useCallback(async () => {
    try {
      const p = await dnsApi.plan(domainId);
      if (!alive.current) return;
      setPlan(p);
      setHint(p.ramp_hint);
      setDays(p.clean_days);
    } catch (e) {
      setError((e as Error).message);
    }
  }, [domainId]);

  const recheck = useCallback(async () => {
    try {
      const c = await dnsApi.check(domainId);
      if (!alive.current) return;
      const m: Record<string, DnsResult> = {};
      c.results.forEach((r) => (m[`${r.kind}|${r.name}`] = r));
      setResults(m);
      setHint(c.ramp_hint);
      setDays(c.clean_days);
      setCheckedAt(new Date());
    } catch (e) {
      setError((e as Error).message);
    }
  }, [domainId]);

  useEffect(() => {
    alive.current = true;
    void loadPlan().then(recheck);
    const t = setInterval(() => void recheck(), pollMs);
    return () => {
      alive.current = false;
      clearInterval(t);
    };
  }, [loadPlan, recheck, pollMs]);

  const act = async (key: string, fn: () => Promise<unknown>) => {
    setBusy(key);
    setError(null);
    try {
      await fn();
      await loadPlan();
      await recheck();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const saveSettings = (patch: Partial<MailSettings>) =>
    act("settings", () => dnsApi.putSettings(domainId, { ...plan!.settings, ...patch }));

  const statusOf = (r: { kind: string; name: string }): CheckStatus | "unknown" => results[`${r.kind}|${r.name}`]?.status ?? "unknown";
  const counts = { pass: 0, warn: 0, fail: 0 };
  plan?.records.forEach((r) => {
    const s = statusOf(r);
    if (s !== "unknown") counts[s]++;
  });
  const next = plan?.keys.find((k) => k.state === "next");
  const active = plan?.keys.find((k) => k.state === "active");
  const extra = Object.values(results).filter((r) => r.kind === "mta-sts-policy");

  return (
    <Sheet variant="soft" sx={{ p: 2, borderRadius: "md", mt: 1.5 }} data-testid={`dns-wizard-${domain}`}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mb: 1 }}>
        <Typography level="title-md">Domain setup: {domain}</Typography>
        <Chip size="sm" color="success" variant="soft">{counts.pass} OK</Chip>
        <Chip size="sm" color="warning" variant="soft">{counts.warn} to check</Chip>
        <Chip size="sm" color="danger" variant="soft">{counts.fail} missing</Chip>
        <Button size="sm" variant="outlined" loading={busy === "check"} onClick={() => act("check", async () => {})} sx={{ ml: "auto" }}>
          Re-check
        </Button>
      </Box>
      <Typography level="body-xs" sx={{ mb: 1 }}>
        Create these records at the DNS provider of {domain}. Checks use public resolvers (1.1.1.1, 8.8.8.8) and refresh every
        15 s.{checkedAt ? ` Last checked ${checkedAt.toLocaleTimeString()}.` : ""}
      </Typography>

      {error && <Alert color="danger" sx={{ mb: 1 }}>{error}</Alert>}
      {plan && plan.mail_backend === "local" && (
        <Alert color="warning" sx={{ mb: 1 }}>
          Mail is not deliverable externally on this deployment (ADRIENT_MAIL_BACKEND=local). Records are still checked so you can
          prepare DNS before connecting a mail server.
        </Alert>
      )}
      {hint && (
        <Alert color="primary" sx={{ mb: 1 }} data-testid="dmarc-hint">
          {hint}
          {plan?.settings.dmarc_policy === "none" && days >= 14 && (
            <Button size="sm" sx={{ ml: 1 }} loading={busy === "settings"} onClick={() => saveSettings({ dmarc_policy: "quarantine" })}>
              Use p=quarantine
            </Button>
          )}
        </Alert>
      )}

      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "sm", mb: 1.5 }}>
        <Typography level="title-sm" sx={{ mb: 0.5 }}>DKIM signing key</Typography>
        {!active && !next && (
          <Button size="sm" loading={busy === "gen"} onClick={() => act("gen", () => dnsApi.generateKey(domainId))}>
            Generate DKIM key
          </Button>
        )}
        {plan?.keys.filter((k) => k.state !== "retired").map((k) => (
          <Box key={k.id} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
            <code>{k.selector}</code>
            <Chip size="sm" variant="soft" color={k.state === "active" ? "success" : "warning"}>{k.state}</Chip>
            <Typography level="body-xs">{k.algo}</Typography>
          </Box>
        ))}
        {next && (
          <Button size="sm" sx={{ mt: 1 }} loading={busy === "act"} onClick={() => act("act", () => dnsApi.activateKey(domainId))}>
            Activate signing
          </Button>
        )}
        {active && !next && (
          <Button size="sm" variant="outlined" sx={{ mt: 1 }} loading={busy === "rot"} onClick={() => act("rot", () => dnsApi.rotateKey(domainId))}>
            Rotate key
          </Button>
        )}
        <Typography level="body-xs" sx={{ mt: 0.5 }}>
          Publish the new key's TXT record first. Activation is accepted only once DNS shows it; a rotation waits 7 days.
        </Typography>
      </Sheet>

      <Box sx={{ overflowX: "auto" }}>
        <Table size="sm" sx={{ minWidth: 760 }}>
          <thead>
            <tr>
              <th style={{ width: 90 }}>Status</th>
              <th style={{ width: 70 }}>Type</th>
              <th>Name</th>
              <th>Value</th>
              <th style={{ width: 70 }} />
            </tr>
          </thead>
          <tbody>
            {plan?.records.map((r) => {
              const res = results[`${r.kind}|${r.name}`];
              const st = statusOf(r);
              return (
                <tr key={`${r.kind}|${r.name}`} data-testid={`rec-${r.kind}`}>
                  <td>
                    <Chip size="sm" color={COLOR[st]} variant="soft" data-status={st}>{LABEL[st]}</Chip>
                  </td>
                  <td>{r.type}</td>
                  <td style={{ wordBreak: "break-all" }}>
                    <code>{r.name}</code>
                    {!r.required && <Typography level="body-xs">optional</Typography>}
                    <CopyButton text={r.name} label={`Copy name ${r.kind}`} />
                  </td>
                  <td style={{ wordBreak: "break-all" }}>
                    <code>{r.value}</code>
                    {res && res.status !== "pass" && (
                      <Box sx={{ mt: 0.5 }} data-testid={`why-${r.kind}`}>
                        <Typography level="body-xs">{res.message}</Typography>
                        <Typography level="body-xs">Found: {res.found?.length ? <code>{res.found.join(" | ")}</code> : "nothing"}</Typography>
                      </Box>
                    )}
                  </td>
                  <td>
                    <CopyButton text={r.value} label={`Copy value ${r.kind}`} />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      </Box>

      {extra.map((r) => (
        <Typography key={r.kind} level="body-sm" sx={{ mt: 1 }}>
          <Chip size="sm" color={COLOR[r.status]} variant="soft">{LABEL[r.status]}</Chip> MTA-STS policy file: {r.message}
        </Typography>
      ))}

      {plan && (
        <Box sx={{ display: "flex", gap: 2, mt: 1.5, flexWrap: "wrap", alignItems: "center" }}>
          <Typography level="body-sm">DMARC policy</Typography>
          <Select
            size="sm"
            value={plan.settings.dmarc_policy}
            onChange={(_, v) => v && saveSettings({ dmarc_policy: v })}
            slotProps={{ button: { "aria-label": "DMARC policy" } }}
          >
            {["none", "quarantine", "reject"].map((p) => (
              <Option key={p} value={p}>p={p}</Option>
            ))}
          </Select>
          <Typography level="body-sm">MTA-STS mode</Typography>
          <Select
            size="sm"
            value={plan.settings.mta_sts_mode}
            onChange={(_, v) => v && saveSettings({ mta_sts_mode: v })}
            slotProps={{ button: { "aria-label": "MTA-STS mode" } }}
          >
            {["none", "testing", "enforce"].map((p) => (
              <Option key={p} value={p}>{p}</Option>
            ))}
          </Select>
        </Box>
      )}
    </Sheet>
  );
}
