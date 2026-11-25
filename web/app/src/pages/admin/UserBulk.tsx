import { useState } from "react";
import { Alert, Box, Button, Typography } from "@mui/joy";
import { deactivateUser, isActive, reactivateUser, type AdminUser } from "./usersApi";

/** What a bulk action would do to a selection: who is changed, who is left alone and why. */
export function planBulk(action: "suspend" | "restore", users: AdminUser[], meEmail: string): { change: AdminUser[]; skipped: { user: AdminUser; why: string }[] } {
  const change: AdminUser[] = [];
  const skipped: { user: AdminUser; why: string }[] = [];
  for (const u of users) {
    if (action === "suspend") {
      if (u.email.toLowerCase() === meEmail.toLowerCase()) skipped.push({ user: u, why: "that is you" });
      else if (!isActive(u.state)) skipped.push({ user: u, why: "already suspended" });
      else change.push(u);
    } else if (isActive(u.state)) skipped.push({ user: u, why: "already active" });
    else change.push(u);
  }
  return { change, skipped };
}

/** Bar shown above the users table while rows are selected. */
export function UserBulkBar({ selected, meEmail, onDone, onClear }: { selected: AdminUser[]; meEmail: string; onDone: (msg: string) => void; onClear: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  if (selected.length === 0) return null;
  async function run(action: "suspend" | "restore") {
    const { change, skipped } = planBulk(action, selected, meEmail);
    if (change.length === 0) return setErr(`Nothing to do: ${skipped.map((s) => `${s.user.email} (${s.why})`).join(", ")}.`);
    const verb = action === "suspend" ? "Suspend" : "Restore";
    if (!window.confirm(`${verb} ${change.length} ${change.length === 1 ? "person" : "people"}?${skipped.length ? `\n\n${skipped.length} left out: ${skipped.map((s) => `${s.user.email} (${s.why})`).join(", ")}` : ""}`)) return;
    setBusy(true);
    setErr("");
    let ok = 0;
    const failed: string[] = [];
    for (const u of change) {
      try {
        await (action === "suspend" ? deactivateUser(u.id) : reactivateUser(u.id));
        ok++;
      } catch (e) {
        failed.push(`${u.email}: ${(e as Error).message}`);
      }
    }
    setBusy(false);
    if (failed.length) setErr(`${ok} done, ${failed.length} failed: ${failed.join("; ")}`);
    onDone(`${verb === "Suspend" ? "Suspended" : "Restored"} ${ok} ${ok === 1 ? "person" : "people"}.`);
  }
  return (
    <Box data-testid="bulk-bar" sx={{ display: "flex", alignItems: "center", gap: 1, p: 1, mb: 1, borderRadius: "md", bgcolor: "primary.softBg", flexWrap: "wrap" }}>
      <Typography level="body-sm" sx={{ fontWeight: 600, mr: 1 }}>{selected.length} selected</Typography>
      <Button size="sm" variant="outlined" color="warning" loading={busy} onClick={() => run("suspend")} data-testid="bulk-suspend">Suspend</Button>
      <Button size="sm" variant="outlined" color="success" loading={busy} onClick={() => run("restore")} data-testid="bulk-restore">Restore</Button>
      <Button size="sm" variant="plain" color="neutral" onClick={onClear}>Clear selection</Button>
      {err && <Alert color="danger" size="sm">{err}</Alert>}
    </Box>
  );
}
