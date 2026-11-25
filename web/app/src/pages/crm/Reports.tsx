import { useCallback, useEffect, useState } from "react";
import { Box, Button, FormControl, FormLabel, Option, Select, Sheet, Stack, Table, Typography } from "@mui/joy";
import * as api2 from "./api2";
import type { Report } from "./types";
import { Empty, ErrorAlert, useErr } from "./ui";
import type { Data } from "./ui";

const REPORTS: [string, string][] = [
  ["velocity", "Pipeline velocity"],
  ["winrate_source", "Win rate by source"],
  ["winrate_owner", "Win rate by owner"],
  ["aging", "Aging deals"],
  ["activity", "Activity by person"],
  ["forecast", "Revenue forecast by team"],
];

export function Reports({ data }: { data: Data }) {
  const [name, setName] = useState("velocity");
  const [days, setDays] = useState("30");
  const [rp, setRp] = useState<Report | null>(null);
  const { err, run } = useErr();
  const extra = name === "activity" ? `?days=${days}` : "";
  const load = useCallback(() => {
    setRp(null);
    void run(() => api2.runReport(name, extra)).then((r) => r && setRp(r));
  }, [name, extra, run]);
  useEffect(load, [load]);
  return (
    <Box>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="flex-end" sx={{ mb: 1.5 }}>
        <Typography level="h4" sx={{ flex: 1 }}>Reports</Typography>
        <FormControl sx={{ width: 240 }}>
          <FormLabel>Report</FormLabel>
          <Select value={name} onChange={(_, v) => setName(v ?? "velocity")} slotProps={{ button: { "data-testid": "crm-report-select" } as never }}>
            {REPORTS.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
          </Select>
        </FormControl>
        {name === "activity" && (
          <FormControl sx={{ width: 130 }}>
            <FormLabel>Period</FormLabel>
            <Select value={days} onChange={(_, v) => setDays(v ?? "30")}><Option value="7">7 days</Option><Option value="30">30 days</Option><Option value="90">90 days</Option></Select>
          </FormControl>
        )}
        <Button variant="outlined" data-testid="crm-report-csv" onClick={() => void run(() => api2.download(`reports/${name}?format=csv${name === "activity" ? `&days=${days}` : ""}`, `crm-${name}.csv`))}>Export CSV</Button>
      </Stack>
      <Typography level="body-sm" sx={{ mb: 1 }}>{rp?.title}{data.me.scope !== "all" && !data.me.is_admin ? " · limited to records you can see" : ""}</Typography>
      <ErrorAlert err={err} />
      {rp && rp.rows.length === 0 && <Empty>No data for this report yet.</Empty>}
      {rp && rp.rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="crm-report-table">
            <thead><tr>{rp.columns.map((c) => <th key={c}>{c}</th>)}</tr></thead>
            <tbody>{rp.rows.map((r, i) => <tr key={i}>{r.map((v, j) => <td key={j}>{v === null ? "" : String(v)}</td>)}</tr>)}</tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}
