import { useEffect, useState } from "react";
import { Box, LinearProgress, Sheet, Typography } from "@mui/joy";

interface Quota {
  used_bytes: number;
  org_used_bytes: number;
  trash_bytes: number;
  version_bytes: number;
  limit_bytes: number;
  breakdown: { category: string; bytes: number; files: number }[];
}

export function fmtBytes(n: number): string {
  if (!n) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${u[i]}`;
}

/** Storage page: usage by file type, trash and version overhead, and the limit when one is configured. */
export function QuotaPanel() {
  const [q, setQ] = useState<Quota | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    fetch("/api/v1/drive/quota", { credentials: "same-origin" })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then(setQ)
      .catch((e) => setErr((e as Error).message));
  }, []);
  if (err) return <Typography color="danger">Could not load storage: {err}</Typography>;
  if (!q) return <Typography sx={{ opacity: 0.6 }}>Loading…</Typography>;
  const total = q.used_bytes + q.trash_bytes + q.version_bytes;
  const rows = [...(q.breakdown ?? [])].sort((a, b) => b.bytes - a.bytes);
  const max = Math.max(1, ...rows.map((r) => r.bytes));
  return (
    <Box data-testid="drive-quota" sx={{ maxWidth: 720 }}>
      <Sheet variant="outlined" sx={{ p: 2.5, borderRadius: "md", mb: 2 }}>
        <Typography level="title-lg">{fmtBytes(q.used_bytes)} used</Typography>
        {q.limit_bytes > 0 ? (
          <>
            <LinearProgress determinate value={Math.min(100, (total / q.limit_bytes) * 100)} sx={{ my: 1 }} />
            <Typography level="body-sm">{fmtBytes(total)} of {fmtBytes(q.limit_bytes)} (files, trash and versions)</Typography>
          </>
        ) : (
          <Typography level="body-sm" sx={{ opacity: 0.7 }}>No storage limit is configured for this workspace.</Typography>
        )}
        <Box sx={{ display: "flex", gap: 3, mt: 1.5, flexWrap: "wrap" }}>
          <Typography level="body-sm">Trash: <b>{fmtBytes(q.trash_bytes)}</b></Typography>
          <Typography level="body-sm">Old versions: <b>{fmtBytes(q.version_bytes)}</b></Typography>
          <Typography level="body-sm">Whole workspace: <b>{fmtBytes(q.org_used_bytes)}</b></Typography>
        </Box>
      </Sheet>
      <Sheet variant="outlined" sx={{ p: 2.5, borderRadius: "md" }}>
        <Typography level="title-md" sx={{ mb: 1 }}>By file type</Typography>
        {rows.length === 0 && <Typography level="body-sm" sx={{ opacity: 0.7 }}>No files yet.</Typography>}
        {rows.map((r) => (
          <Box key={r.category} sx={{ mb: 1.25 }}>
            <Box sx={{ display: "flex", justifyContent: "space-between" }}>
              <Typography level="body-sm">{r.category} <span style={{ opacity: 0.6 }}>({r.files})</span></Typography>
              <Typography level="body-sm">{fmtBytes(r.bytes)}</Typography>
            </Box>
            <LinearProgress determinate value={(r.bytes / max) * 100} size="sm" />
          </Box>
        ))}
      </Sheet>
    </Box>
  );
}
