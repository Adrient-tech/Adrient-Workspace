import { useEffect, useRef, useState } from "react";
import { Alert, Box, Button, Option, Select, Typography } from "@mui/joy";
import * as planApi from "./planApi";
import * as papi from "./api";
import { CSV_FIELDS, csvToDrafts, detectColumns, parseCsv, projectCsv, projectIcs, projectMarkdown, trelloToDrafts, type CsvField, type ImportDraft, type ImportReport } from "./exchange";
import { emptyPlan, type IssuePlan, type Milestone } from "./plan";
import type { Issue, Label, Project, Team } from "./types";

function download(name: string, mime: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: `${mime};charset=utf-8` }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "project";

export function ExchangeTab({ project, issues, plans, onChanged }: { project: Project; issues: Issue[]; plans: Record<string, IssuePlan>; onChanged: () => void }) {
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [teams, setTeams] = useState<Team[]>([]);
  const [teamId, setTeamId] = useState("");
  const [rows, setRows] = useState<string[][] | null>(null);
  const [mapping, setMapping] = useState<(CsvField | null)[]>([]);
  const [trello, setTrello] = useState<{ name: string; drafts: ImportDraft[]; report: ImportReport } | null>(null);
  const [result, setResult] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    papi.listTeams().then((t) => { setTeams(t); setTeamId((cur) => cur || t[0]?.id || ""); }).catch(() => setTeams([]));
  }, []);

  const mine = issues.filter((i) => i.project_id === project.id);

  async function gather() {
    const [milestones, brief, labels] = await Promise.all([
      planApi.listMilestones(project.id).catch(() => [] as Milestone[]),
      planApi.getBrief(project.id).catch(() => ""),
      papi.listLabels().catch(() => [] as Label[]),
    ]);
    return { project, issues: mine, plans, milestones, labels, brief };
  }
  async function exportAs(kind: "md" | "csv" | "ics") {
    setErr("");
    setBusy(true);
    try {
      const x = await gather();
      const base = slug(project.name);
      if (kind === "md") download(`${base}.md`, "text/markdown", projectMarkdown(x));
      else if (kind === "csv") download(`${base}.csv`, "text/csv", projectCsv(x));
      else download(`${base}.ics`, "text/calendar", projectIcs(x));
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function onFile(f: File | undefined) {
    setErr("");
    setResult("");
    setRows(null);
    setTrello(null);
    if (!f) return;
    if (f.size > 5 * 1024 * 1024) { setErr("That file is over 5 MB."); return; }
    const text = await f.text();
    try {
      if (/\.json$/i.test(f.name) || text.trimStart().startsWith("{")) {
        setTrello(trelloToDrafts(JSON.parse(text)));
      } else {
        const r = parseCsv(text);
        if (r.length < 2) throw new Error("The file has no rows below the header.");
        setRows(r);
        setMapping(detectColumns(r[0]));
      }
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  const preview = rows ? csvToDrafts(rows, mapping) : trello ? { drafts: trello.drafts, report: trello.report } : null;
  const titleMapped = mapping.includes("title");

  async function runImport() {
    if (!preview || !teamId) return;
    setBusy(true);
    setErr("");
    let made = 0;
    try {
      const [labels, members, milestones] = await Promise.all([papi.listLabels(), papi.listAssignable(teamId), planApi.listMilestones(project.id)]);
      const labelByName = new Map(labels.map((l) => [l.name.toLowerCase(), l.id]));
      const msByName = new Map(milestones.map((m) => [m.name.toLowerCase(), m.id]));
      for (const d of preview.drafts) {
        const label_ids: string[] = [];
        for (const n of d.labels.slice(0, 8)) {
          let id = labelByName.get(n.toLowerCase());
          if (!id) { id = (await papi.createLabel({ name: n.slice(0, 40) })).id; labelByName.set(n.toLowerCase(), id); }
          label_ids.push(id);
        }
        const assignee = d.assignee ? members.find((m) => m.name.toLowerCase() === d.assignee.toLowerCase() || m.email.toLowerCase() === d.assignee.toLowerCase()) : undefined;
        const issue = await papi.createIssue({
          team_id: teamId, title: d.title, description: d.description, status: d.status, priority: d.priority,
          assignee_id: assignee?.id, label_ids, project_id: project.id,
        });
        made++;
        let ms = "";
        if (d.milestone) {
          const key = d.milestone.toLowerCase();
          if (!msByName.has(key)) msByName.set(key, (await planApi.createMilestone(project.id, { name: d.milestone, due_date: d.due, done: false })).id);
          ms = msByName.get(key) ?? "";
        }
        if (d.start || d.due || ms) await planApi.savePlan(issue.id, { ...emptyPlan(), start_date: d.start, due_date: d.due, milestone_id: ms });
      }
      setResult(`Imported ${made} issue${made === 1 ? "" : "s"} into ${project.name}.`);
      setRows(null);
      setTrello(null);
      if (fileRef.current) fileRef.current.value = "";
      onChanged();
    } catch (e) {
      setErr(`${(e as Error).message}${made ? ` (${made} issues were imported before the error)` : ""}`);
      if (made) onChanged();
    } finally {
      setBusy(false);
    }
  }

  return (
    <Box data-testid="exchange-tab">
      <Typography level="title-sm">Export</Typography>
      <Typography level="body-xs" sx={{ mb: 1 }}>{mine.length} issue{mine.length === 1 ? "" : "s"} in this project.</Typography>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
        <Button size="sm" variant="outlined" disabled={busy} onClick={() => void exportAs("md")} data-testid="export-md">Markdown</Button>
        <Button size="sm" variant="outlined" disabled={busy} onClick={() => void exportAs("csv")} data-testid="export-csv">CSV</Button>
        <Button size="sm" variant="outlined" disabled={busy} onClick={() => void exportAs("ics")} data-testid="export-ics">Calendar (.ics)</Button>
      </Box>

      <Typography level="title-sm" sx={{ mt: 3 }}>Import into this project</Typography>
      <Typography level="body-xs" sx={{ mb: 1 }}>A Trello board export (.json), or a CSV from Asana or any spreadsheet with a header row.</Typography>
      <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
        <input ref={fileRef} type="file" accept=".csv,.json,text/csv,application/json" data-testid="import-file" onChange={(e) => void onFile(e.target.files?.[0])} />
        <Select size="sm" value={teamId} onChange={(_, v) => setTeamId(v ?? "")} placeholder="Team" sx={{ minWidth: 160 }} aria-label="Team for imported issues">
          {teams.map((t) => <Option key={t.id} value={t.id}>{t.name}</Option>)}
        </Select>
      </Box>

      {rows && (
        <Box sx={{ mt: 1.5 }} data-testid="import-mapping">
          <Typography level="body-sm" sx={{ mb: 0.5 }}>Match each column of your file to a field:</Typography>
          {rows[0].map((h, i) => (
            <Box key={i} sx={{ display: "flex", gap: 1, alignItems: "center", py: 0.25 }}>
              <Typography level="body-sm" sx={{ width: 160 }} noWrap title={h}>{h || `Column ${i + 1}`}</Typography>
              <Typography level="body-xs" sx={{ width: 120 }} noWrap>{rows[1]?.[i]}</Typography>
              <Select size="sm" value={mapping[i] ?? ""} onChange={(_, v) => setMapping((cur) => cur.map((m, j) => (j === i ? ((v || null) as CsvField | null) : m)))} sx={{ minWidth: 190 }}>
                <Option value="">Skip this column</Option>
                {CSV_FIELDS.map((f) => <Option key={f.key} value={f.key}>{f.label}</Option>)}
              </Select>
            </Box>
          ))}
          {!titleMapped && <Alert color="warning" sx={{ mt: 1 }}>Choose which column is the task title.</Alert>}
        </Box>
      )}

      {preview && (rows ? titleMapped : true) && (
        <Box sx={{ mt: 1.5 }} data-testid="import-preview">
          <Typography level="body-sm">
            {preview.report.tasks} issue{preview.report.tasks === 1 ? "" : "s"} will be created
            {trello ? ` from "${trello.name}"` : ""}. First: {preview.drafts.slice(0, 3).map((d) => d.title).join(" · ")}
          </Typography>
          {preview.report.notes.map((n) => <Typography key={n} level="body-xs">{n}</Typography>)}
          <Button sx={{ mt: 1 }} size="sm" disabled={busy || !teamId || preview.report.tasks === 0} loading={busy} onClick={() => void runImport()} data-testid="import-run">
            Import {preview.report.tasks} issue{preview.report.tasks === 1 ? "" : "s"}
          </Button>
        </Box>
      )}
      {result && <Alert color="success" sx={{ mt: 1.5 }} data-testid="import-result">{result}</Alert>}
      {err && <Alert color="danger" sx={{ mt: 1.5 }}>{err}</Alert>}
    </Box>
  );
}
