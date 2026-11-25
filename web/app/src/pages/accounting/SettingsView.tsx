import { useEffect, useState } from "react";
import { Box, Button, Checkbox, FormControl, FormHelperText, FormLabel, Input, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { listAudit, saveSettings } from "./api";
import type { AuditRow, Settings } from "./types";
import { ErrorAlert, useErr } from "./ui";

const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

export function SettingsView({ settings, onSaved }: { settings: Settings; onSaved: () => void }) {
  const [s, setS] = useState(settings);
  const [audit, setAudit] = useState<AuditRow[]>([]);
  const [saved, setSaved] = useState(false);
  const { err, run } = useErr();
  useEffect(() => {
    listAudit().then(setAudit).catch(() => undefined);
  }, [settings]);
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 1.5 }}>
        Settings
      </Typography>
      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 620, display: "grid", gap: 1.5 }}>
        <FormControl>
          <FormLabel>Business name</FormLabel>
          <Input value={s.business_name} onChange={(e) => setS({ ...s, business_name: e.target.value })} data-testid="acct-set-name" />
        </FormControl>
        <FormControl>
          <FormLabel>GSTIN</FormLabel>
          <Input value={s.gstin} onChange={(e) => setS({ ...s, gstin: e.target.value })} />
        </FormControl>
        <Box sx={{ display: "flex", gap: 1.5 }}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Financial year starts in</FormLabel>
            <Select value={s.fy_start_month} onChange={(_, v) => setS({ ...s, fy_start_month: Number(v) || 4 })}>
              {MONTHS.map((m, i) => (
                <Option key={m} value={i + 1}>
                  {m}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl sx={{ width: 120 }}>
            <FormLabel>Currency</FormLabel>
            <Input value={s.currency} onChange={(e) => setS({ ...s, currency: e.target.value })} />
          </FormControl>
        </Box>
        <Checkbox label="Sales are within the home state (split GST into CGST and SGST; untick for IGST)" checked={s.intrastate} onChange={(e) => setS({ ...s, intrastate: e.target.checked })} />
        <Checkbox label="Hold customer payments in Undeposited Funds until I deposit them in the bank" checked={s.use_undeposited} onChange={(e) => setS({ ...s, use_undeposited: e.target.checked })} slotProps={{ input: { "data-testid": "acct-set-undeposited" } as never }} />
        <FormControl>
          <FormLabel>Bill approval limit</FormLabel>
          <Input type="number" value={s.approval_limit || ""} onChange={(e) => setS({ ...s, approval_limit: Number(e.target.value) || 0 })} slotProps={{ input: { "data-testid": "acct-set-approval" } }} />
          <FormHelperText>Bills above this amount wait for someone with the accounting approver role (not the person who entered it). Leave blank for no approval step.</FormHelperText>
        </FormControl>
        <FormControl>
          <FormLabel>Closing date (lock books)</FormLabel>
          <Input type="date" value={s.closing_date} onChange={(e) => setS({ ...s, closing_date: e.target.value })} data-testid="acct-set-closing" />
          <FormHelperText>No entry dated on or before this day can be added, changed or deleted, including by Sync from apps. Clear it to unlock.</FormHelperText>
        </FormControl>
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <Button
            data-testid="acct-set-save"
            onClick={async () => {
              const r = await run(() => saveSettings(s));
              if (r) {
                setSaved(true);
                onSaved();
                setTimeout(() => setSaved(false), 2500);
              }
            }}
          >
            Save settings
          </Button>
          {saved && <Typography level="body-sm" color="success">Saved</Typography>}
        </Box>
      </Sheet>
      <Typography level="title-md" sx={{ mt: 3, mb: 1 }}>
        Audit log
      </Typography>
      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", maxHeight: 360 }}>
        <Table size="sm" data-testid="acct-audit">
          <thead>
            <tr>
              <th style={{ width: 170 }}>When</th>
              <th style={{ width: 90 }}>Action</th>
              <th style={{ width: 130 }}>Record</th>
              <th>Detail</th>
              <th style={{ width: 90 }}>Change</th>
            </tr>
          </thead>
          <tbody>
            {audit.map((a) => (
              <tr key={a.id}>
                <td>{a.at.replace("T", " ").replace("Z", "")}</td>
                <td>{a.action}</td>
                <td>{a.entity}</td>
                <td>{a.detail}</td>
                <td>
                  {(a.before !== undefined || a.after !== undefined) ? (
                    <details>
                      <summary>before / after</summary>
                      <pre style={{ fontSize: 11, maxWidth: 520, overflow: "auto", margin: 0 }} data-testid="acct-audit-diff">
                        {JSON.stringify({ before: a.before ?? null, after: a.after ?? null }, null, 1)}
                      </pre>
                    </details>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </Sheet>
    </Box>
  );
}
