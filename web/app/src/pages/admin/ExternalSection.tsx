// External collaboration: who may invite guests, which partner domains are
// allowed or blocked, and what guests can see in the directory.
import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Chip, ChipDelete, FormControl, FormLabel, Input, Option, Select, Sheet, Switch, Typography } from "@mui/joy";
import { externalApi, type ExternalPolicy, type Partner } from "./controlsApi";

export function ExternalSection() {
  const [policy, setPolicy] = useState<ExternalPolicy | null>(null);
  const [partners, setPartners] = useState<Partner[]>([]);
  const [domain, setDomain] = useState("");
  const [effect, setEffect] = useState<"allow" | "block">("block");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [p, l] = await Promise.all([externalApi.policy(), externalApi.partners()]);
      setPolicy(p);
      setPartners(l.partners);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  async function save(next: ExternalPolicy) {
    setError(null);
    try {
      setPolicy(await externalApi.savePolicy(next));
      setNotice("External collaboration policy saved.");
    } catch (e) {
      setError((e as Error).message);
    }
  }

  async function addPartner() {
    setError(null);
    try {
      await externalApi.addPartner({ domain, effect, note: "" });
      setDomain("");
      setNotice(`${effect === "block" ? "Blocked" : "Allowed"} ${domain.trim().toLowerCase()}.`);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  async function removePartner(d: string) {
    try {
      await externalApi.removePartner(d);
      await load();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Box data-testid="external-section">
      <Typography level="h4">External collaboration</Typography>
      <Typography level="body-sm" sx={{ opacity: 0.7, mb: 2 }}>
        Controls who outside your organization can be invited, shared with, chatted with and met. Sharing
        in Drive, Chat and Meet all follow this one policy.
      </Typography>
      {notice && (
        <Alert color="success" variant="soft" sx={{ mb: 1.5 }}>
          {notice}
        </Alert>
      )}
      {error && (
        <Alert color="danger" variant="soft" sx={{ mb: 1.5 }}>
          {error}
        </Alert>
      )}
      {policy && (
        <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2, display: "grid", gap: 1.5, maxWidth: 640 }}>
          <FormControl size="sm">
            <FormLabel>Who can invite guests</FormLabel>
            <Select size="sm" value={policy.guest_invites} onChange={(_, v) => v && void save({ ...policy, guest_invites: v })} data-testid="ext-invites">
              <Option value="none">Nobody</Option>
              <Option value="admins">Admins</Option>
              <Option value="admins_and_sponsors">Admins and sponsors</Option>
              <Option value="members">Members</Option>
              <Option value="everyone">Everyone</Option>
            </Select>
          </FormControl>
          <FormControl size="sm">
            <FormLabel>Partner domains</FormLabel>
            <Select size="sm" value={policy.domain_mode} onChange={(_, v) => v && void save({ ...policy, domain_mode: v })} data-testid="ext-mode">
              <Option value="allow_all">Allow any domain (except blocked)</Option>
              <Option value="allowlist">Only allowed domains</Option>
              <Option value="blocklist">Block listed domains, and personal mail unless allowed</Option>
            </Select>
          </FormControl>
          <FormControl size="sm">
            <FormLabel>What guests see in the directory</FormLabel>
            <Select size="sm" value={policy.guest_visibility} onChange={(_, v) => v && void save({ ...policy, guest_visibility: v })} data-testid="ext-visibility">
              <Option value="limited">Only their sponsor, people who shared with them and their groups</Option>
              <Option value="directory">The whole directory</Option>
            </Select>
          </FormControl>
          <FormControl size="sm" orientation="horizontal" sx={{ gap: 1 }}>
            <Switch checked={policy.require_mfa_for_guests} onChange={(e) => void save({ ...policy, require_mfa_for_guests: e.target.checked })} />
            <FormLabel>Require multi-factor sign-in for guests (applied by the sign-in policy)</FormLabel>
          </FormControl>
        </Sheet>
      )}
      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 640 }}>
        <Typography level="title-sm" sx={{ mb: 1 }}>
          Partner domains
        </Typography>
        <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mb: 1.5 }}>
          {partners.length === 0 && (
            <Typography level="body-sm" sx={{ opacity: 0.6 }}>
              No partner domains yet.
            </Typography>
          )}
          {partners.map((p) => (
            <Chip
              key={p.domain}
              variant="soft"
              color={p.effect === "block" ? "danger" : "success"}
              data-testid={`partner-${p.domain}`}
              endDecorator={<ChipDelete onDelete={() => void removePartner(p.domain)} />}
            >
              {p.effect === "block" ? "Blocked" : "Allowed"} · {p.domain}
            </Chip>
          ))}
        </Box>
        <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <Input size="sm" placeholder="partner.com" value={domain} onChange={(e) => setDomain(e.target.value)} sx={{ flex: 1 }} slotProps={{ input: { "data-testid": "partner-domain" } as never }} />
          <Select size="sm" value={effect} onChange={(_, v) => v && setEffect(v)} sx={{ width: 110 }} data-testid="partner-effect">
            <Option value="allow">Allow</Option>
            <Option value="block">Block</Option>
          </Select>
          <Button size="sm" disabled={!domain.trim()} onClick={addPartner} data-testid="partner-add">
            Add
          </Button>
        </Box>
      </Sheet>
    </Box>
  );
}
