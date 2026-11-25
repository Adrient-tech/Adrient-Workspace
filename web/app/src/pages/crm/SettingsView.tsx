import { useRef, useState } from "react";
import { Box, Button, FormControl, FormLabel, IconButton, Input, Sheet, Stack, Typography } from "@mui/joy";
import * as api from "./api";
import { parseDealsCSV, toCSV, allStages } from "./logic";
import type { Stage } from "./types";
import { ErrorAlert, downloadCSV, useErr } from "./ui";
import type { Data } from "./ui";

const slug = (n: string) => n.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "stage";

export function SettingsView({ data }: { data: Data }) {
  const s = data.settings;
  const [stages, setStages] = useState<Stage[]>(s.stages);
  const [currency, setCurrency] = useState(s.currency);
  const [goal, setGoal] = useState(s.monthly_goal);
  const [stale, setStale] = useState(s.stale_days);
  const [cold, setCold] = useState(s.cold_days);
  const [weekly, setWeekly] = useState(s.weekly_target);
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();
  const file = useRef<HTMLInputElement>(null);

  const move = (i: number, d: number) => {
    const next = [...stages];
    const j = i + d;
    if (j < 0 || j >= next.length) return;
    [next[i], next[j]] = [next[j], next[i]];
    setStages(next);
  };
  const addStage = () => {
    let id = "stage";
    for (let n = stages.length + 1; stages.some((x) => x.id === id) || id === "stage"; n++) id = `stage-${n}`;
    setStages([...stages, { id, name: `Stage ${stages.length + 1}` }]);
  };

  const save = async () => {
    // New stages get an id from their name once; existing ids never change so deals keep their stage.
    const known = new Set(s.stages.map((x) => x.id));
    const used = new Set<string>();
    const fixed = stages.map((st) => {
      let id = st.id;
      if (!known.has(id)) {
        id = slug(st.name);
        while (used.has(id) || id === "won" || id === "lost") id += "-x";
      }
      used.add(id);
      return { id, name: st.name.trim() };
    });
    const r = await run(() => api.saveSettings({ stages: fixed, currency, monthly_goal: goal, stale_days: stale, cold_days: cold, weekly_target: weekly }));
    if (r) {
      setMsg("Saved");
      data.reload();
    }
  };

  const importDeals = async (f: File) => {
    const rows = parseDealsCSV(await f.text());
    if (rows.length === 0) {
      setMsg("No deals found. Expected columns: title, company, value, stage, expected_close, source.");
      return;
    }
    let made = 0;
    await run(async () => {
      const companies = new Map(data.companies.map((c) => [c.name.toLowerCase(), c.id]));
      for (const r of rows) {
        let companyId = "";
        if (r.company) {
          companyId = companies.get(r.company.toLowerCase()) ?? "";
          if (!companyId) {
            const c = await api.saveCompany("", { name: r.company });
            companyId = c.id;
            companies.set(r.company.toLowerCase(), c.id);
          }
        }
        const stage = allStages(s).some((st) => st.id === r.stage) ? r.stage : s.stages[0].id;
        await api.saveDeal("", { title: r.title, company_id: companyId, value: r.value, stage, expected_close: r.expected_close, source: r.source });
        made++;
      }
    });
    setMsg(`Imported ${made} of ${rows.length} deals.`);
    data.reload();
  };

  const exportDeals = () =>
    downloadCSV(
      "crm-deals.csv",
      toCSV([
        ["title", "company", "value", "currency", "billing", "stage", "expected_close", "source", "close_reason"],
        ...data.deals.map((d) => [d.title, data.companies.find((c) => c.id === d.company_id)?.name ?? "", d.value, d.currency, d.billing, d.stage, d.expected_close ?? "", d.source, d.close_reason]),
      ]),
    );

  return (
    <Box data-testid="crm-settings">
      <Typography level="h4" sx={{ mb: 1.5 }}>
        Settings
      </Typography>
      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", mb: 2, maxWidth: 640 }}>
        <Typography level="title-md" sx={{ mb: 1 }}>
          Funnel stages
        </Typography>
        <Typography level="body-xs" sx={{ mb: 1 }}>
          Up to 8 open stages. Won and Lost are always the last two. Deals in a removed stage move to the first stage.
        </Typography>
        <Stack gap={0.5}>
          {stages.map((st, i) => (
            <Stack key={st.id} direction="row" gap={0.5}>
              <Input size="sm" sx={{ flex: 1 }} value={st.name} onChange={(e) => setStages(stages.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} slotProps={{ input: { "aria-label": `Stage ${i + 1} name` } }} />
              <IconButton size="sm" aria-label="Move up" onClick={() => move(i, -1)}>
                ↑
              </IconButton>
              <IconButton size="sm" aria-label="Move down" onClick={() => move(i, 1)}>
                ↓
              </IconButton>
              <IconButton size="sm" aria-label="Remove stage" disabled={stages.length <= 1} onClick={() => setStages(stages.filter((_, j) => j !== i))}>
                ×
              </IconButton>
            </Stack>
          ))}
          <Button size="sm" variant="soft" sx={{ alignSelf: "flex-start" }} disabled={stages.length >= 8} onClick={addStage} data-testid="crm-add-stage">
            Add stage
          </Button>
        </Stack>
        <Stack direction="row" gap={1} flexWrap="wrap" sx={{ mt: 2 }}>
          <FormControl sx={{ width: 110 }}>
            <FormLabel>Currency</FormLabel>
            <Input value={currency} onChange={(e) => setCurrency(e.target.value.toUpperCase())} slotProps={{ input: { maxLength: 3, "data-testid": "crm-currency" } }} />
          </FormControl>
          <FormControl sx={{ width: 170 }}>
            <FormLabel>Monthly revenue goal</FormLabel>
            <Input type="number" value={goal} onChange={(e) => setGoal(Number(e.target.value))} slotProps={{ input: { min: 0, "data-testid": "crm-goal" } }} />
          </FormControl>
          <FormControl sx={{ width: 130 }}>
            <FormLabel>Stalled after (days)</FormLabel>
            <Input type="number" value={stale} onChange={(e) => setStale(Number(e.target.value))} slotProps={{ input: { min: 0 } }} />
          </FormControl>
          <FormControl sx={{ width: 130 }}>
            <FormLabel>Cold after (days)</FormLabel>
            <Input type="number" value={cold} onChange={(e) => setCold(Number(e.target.value))} slotProps={{ input: { min: 0 } }} />
          </FormControl>
          <FormControl sx={{ width: 130 }}>
            <FormLabel>Weekly touches</FormLabel>
            <Input type="number" value={weekly} onChange={(e) => setWeekly(Number(e.target.value))} slotProps={{ input: { min: 0 } }} />
          </FormControl>
        </Stack>
        <ErrorAlert err={err} />
        <Stack direction="row" gap={1} alignItems="center" sx={{ mt: 2 }}>
          <Button onClick={save} data-testid="crm-settings-save">
            Save settings
          </Button>
          {msg && <Typography level="body-sm">{msg}</Typography>}
        </Stack>
      </Sheet>
      <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", maxWidth: 640 }}>
        <Typography level="title-md" sx={{ mb: 1 }}>
          CSV
        </Typography>
        <Stack direction="row" gap={1}>
          <Button size="sm" variant="outlined" onClick={exportDeals}>
            Export deals
          </Button>
          <Button size="sm" variant="outlined" onClick={() => file.current?.click()}>
            Import deals
          </Button>
          <input
            ref={file}
            type="file"
            accept=".csv,text/csv"
            hidden
            data-testid="crm-import-file"
            onChange={(e) => {
              const f = e.target.files?.[0];
              e.target.value = "";
              if (f) void importDeals(f);
            }}
          />
        </Stack>
        <Typography level="body-xs" sx={{ mt: 1 }}>
          Import columns: title, company, value, stage, expected_close (YYYY-MM-DD), source. Leads and companies export from their own pages.
        </Typography>
      </Sheet>
    </Box>
  );
}
