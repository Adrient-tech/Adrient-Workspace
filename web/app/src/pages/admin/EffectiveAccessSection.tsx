// Effective access viewer: what can this user open and do? Calls the same
// resolver the server enforces with, with optional "what if" IP and user type.
import { useEffect, useState } from "react";
import { Alert, Box, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Typography } from "@mui/joy";
import { apps as catalog } from "../../catalog/apps";
import type { Subject } from "./accessApi";
import { SubjectPicker } from "./SubjectPicker";
import { controlsApi, showValue, type ControlResult, type Effective } from "./controlsApi";
import { ControlDefsProvider, useDef } from "./controlDefs";
import { ruleLabel } from "./ControlsSection";

const appName = (id: string) => catalog.find((a) => a.id === id)?.name ?? id;

export function EffectiveAccessSection() {
  const [user, setUser] = useState<Subject | null>(null);
  const [ip, setIp] = useState("");
  const [utype, setUtype] = useState("");
  const [data, setData] = useState<Effective | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!user) {
      setData(null);
      return;
    }
    let alive = true;
    const t = setTimeout(() => {
      controlsApi
        .effective(user.id, { ip: ip.trim(), userType: utype })
        .then((d) => {
          if (alive) {
            setData(d);
            setErr(null);
          }
        })
        .catch((e: Error) => alive && setErr(e.message));
    }, 250);
    return () => {
      alive = false;
      clearTimeout(t);
    };
  }, [user, ip, utype]);

  return (
    <ControlDefsProvider>
      <Box data-testid="effective-access">
        <Typography level="h4">Effective access</Typography>
        <Typography level="body-sm" sx={{ opacity: 0.7, mb: 2 }}>
          Pick a person to see which apps they can open and the value of every control, with the rule that
          decided it. Try a different IP address or user type to see how the answer changes.
        </Typography>
        <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "flex-end", mb: 2 }}>
          <FormControl size="sm">
            <FormLabel>User</FormLabel>
            <SubjectPicker type="user" value={user} onChange={setUser} placeholder="Pick a user" testId="eff-user" />
          </FormControl>
          <FormControl size="sm">
            <FormLabel>What if: IP address</FormLabel>
            <Input size="sm" placeholder="203.0.113.7" value={ip} onChange={(e) => setIp(e.target.value)} slotProps={{ input: { "data-testid": "eff-ip" } as never }} />
          </FormControl>
          <FormControl size="sm">
            <FormLabel>What if: user type</FormLabel>
            <Select size="sm" value={utype} onChange={(_, v) => setUtype(v ?? "")} sx={{ width: 140 }} data-testid="eff-type">
              <Option value="">Actual</Option>
              <Option value="member">Member</Option>
              <Option value="external">External</Option>
              <Option value="guest">Guest</Option>
            </Select>
          </FormControl>
        </Box>
        {err && (
          <Alert color="danger" variant="soft" sx={{ mb: 1.5 }}>
            {err}
          </Alert>
        )}
        {data && (
          <Box data-testid="eff-result">
            <Typography level="body-sm" sx={{ mb: 1 }}>
              {data.user.name || data.user.email} · {data.user.user_type}
              {data.simulated.user_type !== data.user.user_type ? ` (shown as ${data.simulated.user_type})` : ""}
              {data.simulated.ip ? ` from ${data.simulated.ip}` : ""} · roles: {data.roles.join(", ") || "member"}
            </Typography>
            <Box sx={{ display: "grid", gap: 1 }}>
              {data.apps
                .filter((a) => a.allowed || a.controls.length > 0)
                .map((a) => (
                  <Sheet key={a.id} variant="outlined" sx={{ p: 1.25, borderRadius: "md" }} data-testid={`eff-app-${a.id}`}>
                    <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                      <Typography level="title-sm" sx={{ width: 130 }}>
                        {appName(a.id)}
                      </Typography>
                      <Chip size="sm" color={a.allowed ? "success" : "danger"} variant="soft" data-testid={`eff-open-${a.id}-${a.allowed ? "yes" : "no"}`}>
                        {a.allowed ? "Can open" : "Cannot open"}
                      </Chip>
                      <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                        {a.reason}
                      </Typography>
                    </Box>
                    {a.allowed && a.controls.length > 0 && (
                      <Box sx={{ mt: 0.75, display: "grid", gap: 0.5 }}>
                        {a.controls.map((c) => (
                          <ControlLine key={c.key} c={c} />
                        ))}
                      </Box>
                    )}
                  </Sheet>
                ))}
              {data.shared.length > 0 && (
                <Sheet variant="outlined" sx={{ p: 1.25, borderRadius: "md" }}>
                  <Typography level="title-sm">All apps</Typography>
                  {data.shared.map((c) => (
                    <ControlLine key={c.key} c={c} />
                  ))}
                </Sheet>
              )}
            </Box>
          </Box>
        )}
      </Box>
    </ControlDefsProvider>
  );
}

function ControlLine({ c }: { c: ControlResult }) {
  const def = useDef(c.key);
  const why =
    c.source === "rule" && c.winner
      ? `rule: ${ruleLabel(c.winner.rule)}`
      : c.source === "error"
        ? "could not be read, restricted"
        : "default";
  const notMet = (c.rules ?? []).filter((r) => r.status === "condition_not_met");
  return (
    <Box sx={{ display: "flex", gap: 1, alignItems: "baseline", flexWrap: "wrap" }} data-testid={`eff-control-${c.key}`}>
      <Typography level="body-sm" sx={{ width: 260 }}>
        {def?.label ?? c.key}
      </Typography>
      <Chip size="sm" variant="soft" color={c.value === false || c.value === "off" ? "warning" : "neutral"} data-testid={`eff-value-${c.key}`}>
        {showValue(def, c.value)}
      </Chip>
      <Typography level="body-xs" sx={{ opacity: 0.7 }}>
        {why}
        {notMet.length > 0 ? ` · not applied: ${notMet.map((r) => r.why).join("; ")}` : ""}
      </Typography>
    </Box>
  );
}
