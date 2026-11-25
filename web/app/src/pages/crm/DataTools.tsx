import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Checkbox, FormControl, FormLabel, IconButton, Input, Option, Radio, RadioGroup, Select, Sheet, Stack, Tab, TabList, TabPanel, Table, Tabs, Typography } from "@mui/joy";
import * as api2 from "./api2";
import { Dialog, Empty, ErrorAlert, downloadCSV, useErr } from "./ui";
import type { Data } from "./ui";
import type { FieldDef } from "./types";

const SOURCES: [string, string, string][] = [
  ["zoho", "Zoho CRM", "In Zoho CRM open the module (Leads, Accounts or Deals), choose Export, and pick CSV."],
  ["hubspot", "HubSpot", "In HubSpot open Contacts, Companies or Deals, click Export, and choose CSV. Include the Record ID column."],
  ["salesforce", "Salesforce", "In Salesforce run a report or use Data Export to get Leads, Accounts or Opportunities as CSV. Keep the Id column."],
  ["csv", "Any CSV", "Any spreadsheet saved as CSV with a header row. You choose which column is which."],
];
const ENTITIES: [string, string][] = [["lead", "Leads and contacts"], ["company", "Companies"], ["deal", "Deals"]];

interface Preview {
  headers: string[];
  suggested: Record<string, string>;
  sample: string[][];
  total: number;
  targets: string[];
  custom_fields: FieldDef[];
}
interface Report {
  source: string;
  entity: string;
  dry_run: boolean;
  total: number;
  created: number;
  updated: number;
  skipped: number;
  errors: { row: number; message: string }[];
}

export function DataTools({ data }: { data: Data }) {
  return (
    <Box>
      <Typography level="h4" sx={{ mb: 1 }}>Data tools</Typography>
      <Tabs defaultValue="import" sx={{ bgcolor: "transparent" }}>
        <TabList>
          <Tab value="import" data-testid="crm-data-tab-import">Import</Tab>
          <Tab value="export" data-testid="crm-data-tab-export">Export</Tab>
          <Tab value="dupes" data-testid="crm-data-tab-dupes">Duplicates</Tab>
          <Tab value="fields" data-testid="crm-data-tab-fields">Custom fields</Tab>
        </TabList>
        <TabPanel value="import">{data.me.is_admin ? <ImportWizard data={data} /> : <Typography level="body-sm">Importing is limited to CRM administrators.</Typography>}</TabPanel>
        <TabPanel value="export"><Export /></TabPanel>
        <TabPanel value="dupes"><Duplicates data={data} /></TabPanel>
        <TabPanel value="fields"><FieldsAdmin data={data} /></TabPanel>
      </Tabs>
    </Box>
  );
}

function ImportWizard({ data }: { data: Data }) {
  const [source, setSource] = useState("csv");
  const [entity, setEntity] = useState("lead");
  const [csv, setCsv] = useState("");
  const [fileName, setFileName] = useState("");
  const [pv, setPv] = useState<Preview | null>(null);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [rep, setRep] = useState<Report | null>(null);
  const [busy, setBusy] = useState(false);
  const { err, run } = useErr();
  const help = SOURCES.find((s) => s[0] === source)?.[2];

  const preview = async (text: string, src = source, ent = entity) => {
    const r = await run(() => api2.post<Preview>("import/preview", { source: src, entity: ent, csv: text }));
    if (r) {
      setPv(r);
      setMapping(r.suggested);
      setRep(null);
    }
  };
  const go = async (dry: boolean) => {
    setBusy(true);
    const r = await run(() => api2.post<Report>("import", { source, entity, csv, mapping, dry_run: dry }));
    setBusy(false);
    if (r) {
      setRep(r);
      if (!dry) data.reload();
    }
  };
  const targetOptions: [string, string][] = pv ? [...pv.targets.map((t) => [t, t.replace(/_/g, " ")] as [string, string]), ...pv.custom_fields.map((d) => [`custom:${d.key}`, `${d.label} (custom)`] as [string, string])] : [];
  const mapped = Object.values(mapping).filter(Boolean).length;

  return (
    <Stack gap={1.5} data-testid="crm-import">
      <Typography level="body-sm">Bring your data in from another CRM. You see how columns map, run a dry run that changes nothing, then import. Running the same file again updates records instead of creating duplicates.</Typography>
      <Stack direction="row" gap={1} flexWrap="wrap">
        <FormControl sx={{ width: 200 }}><FormLabel>Coming from</FormLabel>
          <Select value={source} onChange={(_, v) => { setSource(v ?? "csv"); setPv(null); setRep(null); }} slotProps={{ button: { "data-testid": "crm-import-source" } as never }}>
            {SOURCES.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
          </Select></FormControl>
        <FormControl sx={{ width: 200 }}><FormLabel>What is in the file</FormLabel>
          <Select value={entity} onChange={(_, v) => { setEntity(v ?? "lead"); setPv(null); setRep(null); }} slotProps={{ button: { "data-testid": "crm-import-entity" } as never }}>
            {ENTITIES.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
          </Select></FormControl>
        <FormControl><FormLabel>CSV file</FormLabel>
          <input type="file" accept=".csv,text/csv" aria-label="CSV file" data-testid="crm-import-file" onChange={async (e) => {
            const f = e.target.files?.[0];
            if (!f) return;
            const text = await f.text();
            setCsv(text);
            setFileName(f.name);
            await preview(text);
          }} /></FormControl>
      </Stack>
      <Typography level="body-xs">{help}</Typography>
      <ErrorAlert err={err} />
      {pv && (
        <>
          <Typography level="title-sm">Match columns ({fileName}, {pv.total} rows)</Typography>
          <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
            <Table data-testid="crm-import-mapping">
              <thead><tr><th>Column in your file</th><th>Example</th><th>Goes to</th></tr></thead>
              <tbody>
                {pv.headers.map((h, i) => (
                  <tr key={h + i}>
                    <td>{h}</td>
                    <td>{pv.sample[0]?.[i] ?? ""}</td>
                    <td>
                      <Select size="sm" value={mapping[h] ?? ""} onChange={(_, v) => setMapping((m) => { const n = { ...m }; if (v) n[h] = v; else delete n[h]; return n; })} sx={{ width: 220 }} aria-label={`Map ${h}`} slotProps={{ button: { "data-testid": `crm-map-${h}` } as never }}>
                        <Option value="">Skip this column</Option>
                        {targetOptions.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
                      </Select>
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Sheet>
          <Stack direction="row" gap={1}>
            <Button variant="outlined" disabled={busy || mapped === 0} onClick={() => go(true)} data-testid="crm-import-dry">Dry run (changes nothing)</Button>
            <Button disabled={busy || mapped === 0 || !rep || !rep.dry_run || rep.errors.length === rep.total} onClick={() => go(false)} data-testid="crm-import-run">Import now</Button>
          </Stack>
        </>
      )}
      {rep && (
        <Alert color={rep.errors.length ? "warning" : "success"} data-testid="crm-import-report" sx={{ display: "block" }}>
          <Typography level="title-sm">{rep.dry_run ? "Dry run result (nothing was saved)" : "Import finished"}</Typography>
          <Typography level="body-sm" data-testid="crm-import-counts">
            {rep.total} rows · {rep.created} {rep.dry_run ? "would be created" : "created"} · {rep.updated} {rep.dry_run ? "would be updated" : "updated"} · {rep.errors.length} with errors
          </Typography>
          {rep.errors.slice(0, 8).map((e) => <Typography key={e.row} level="body-xs">Row {e.row}: {e.message}</Typography>)}
          {rep.errors.length > 0 && (
            <Button size="sm" variant="soft" sx={{ mt: 0.5 }} data-testid="crm-import-errors" onClick={() => downloadCSV("crm-import-errors.csv", "row,error\n" + rep.errors.map((e) => `${e.row},"${e.message.replace(/"/g, '""')}"`).join("\n"))}>Download error report</Button>
          )}
        </Alert>
      )}
    </Stack>
  );
}

function Export() {
  const { err, run } = useErr();
  const items: [string, string][] = [["leads", "Leads and contacts"], ["companies", "Companies"], ["deals", "Deals"], ["activities", "Activities"]];
  return (
    <Stack gap={1.5} data-testid="crm-export">
      <Typography level="body-sm">Download your CRM data as CSV, custom fields included. The files contain only the records you are allowed to see. Cells that could run as spreadsheet formulas are neutralised.</Typography>
      <Stack direction="row" gap={1} flexWrap="wrap">
        {items.map(([k, l]) => <Button key={k} variant="outlined" data-testid={`crm-export-${k}`} onClick={() => void run(() => api2.download(`export/${k}.csv`, `crm-${k}.csv`))}>{l}</Button>)}
        <Button data-testid="crm-export-all" onClick={() => void run(() => api2.download("export/all.zip", "crm-export.zip"))}>Everything (zip)</Button>
      </Stack>
      <ErrorAlert err={err} />
    </Stack>
  );
}

interface Group {
  kind: "person" | "company";
  reason: string;
  ids: string[];
  names: string[];
}

function Duplicates({ data }: { data: Data }) {
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [merging, setMerging] = useState<Group | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<Group>("duplicates", "groups")).then((g) => g && setGroups(g)); }, [run]);
  useEffect(load, [load]);
  return (
    <Stack gap={1.5} data-testid="crm-dupes">
      <Typography level="body-sm">Leads that share an email or phone number, and companies with the same name or website. Merging moves deals, follow-ups, activity and mail to the record you keep. The other contact goes to the Contacts trash, so it can be restored.</Typography>
      <ErrorAlert err={err} />
      {groups && groups.length === 0 && <Empty>No duplicates found.</Empty>}
      {groups?.map((g) => (
        <Stack key={g.ids.join()} direction="row" gap={1} alignItems="center" data-testid="crm-dupe-group">
          <Typography level="body-sm" sx={{ flex: 1 }}>{g.kind === "person" ? "Leads" : "Companies"}: {g.names.join(" / ")} <Typography level="body-xs" component="span">(same {g.reason.replace("+", " and ")})</Typography></Typography>
          <Button size="sm" onClick={() => setMerging(g)} data-testid="crm-dupe-merge">Merge</Button>
        </Stack>
      ))}
      {merging && <MergeDialog group={merging} onClose={() => setMerging(null)} onDone={() => { setMerging(null); load(); data.reload(); }} />}
    </Stack>
  );
}

function MergeDialog({ group, onClose, onDone }: { group: Group; onClose: () => void; onDone: () => void }) {
  const [winner, setWinner] = useState(group.ids[0]);
  const { err, run } = useErr();
  return (
    <Dialog title="Merge duplicates" onClose={onClose} width={480}>
      <Stack gap={1.5}>
        <Typography level="body-sm">Choose the record to keep. The others are merged into it.</Typography>
        <RadioGroup value={winner} onChange={(e) => setWinner(e.target.value)}>
          {group.ids.map((id, i) => (
            <Radio key={id} value={id} label={group.names[i]} data-testid={`crm-merge-keep-${i}`} />
          ))}
        </RadioGroup>
        <ErrorAlert err={err} />
        <Button data-testid="crm-merge-confirm" onClick={async () => {
          for (const id of group.ids.filter((x) => x !== winner)) {
            if (!(await run(() => api2.post("merge", { kind: group.kind, winner, loser: id })))) return;
          }
          onDone();
        }}>Merge into the selected record</Button>
      </Stack>
    </Dialog>
  );
}

const TYPES = ["text", "number", "date", "select", "multiselect", "url", "user"];

function FieldsAdmin({ data }: { data: Data }) {
  const [f, setF] = useState({ entity: "lead", label: "", key: "", type: "text", options: "", required: false });
  const { err, run } = useErr();
  const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "").replace(/^(\d)/, "f_$1").slice(0, 40);
  return (
    <Stack gap={1.5} data-testid="crm-fields-admin">
      <Typography level="body-sm">Custom fields appear in the lead, company and deal forms, in lists and filters, in saved views, and in imports and exports. "Lead" and "contact" fields are the same record kind and share keys.</Typography>
      {data.fields.length === 0 ? <Typography level="body-sm">No custom fields yet.</Typography> : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table data-testid="crm-fields-table">
            <thead><tr><th>On</th><th>Label</th><th>Key</th><th>Type</th><th>Required</th><th /></tr></thead>
            <tbody>
              {data.fields.map((d) => (
                <tr key={d.id}><td>{d.entity}</td><td>{d.label}</td><td>{d.key}</td><td>{d.type}{d.options.length ? ` (${d.options.join(", ")})` : ""}</td><td>{d.required ? "Yes" : ""}</td>
                  <td>{data.me.is_admin && <IconButton size="sm" aria-label={`Delete field ${d.label}`} onClick={async () => { if (window.confirm(`Delete "${d.label}" and its values?`) && (await run(() => api2.remove("fields", d.id)))) data.reload(); }}>×</IconButton>}</td></tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {data.me.is_admin && (
        <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md" }}>
          <Stack direction="row" gap={1} flexWrap="wrap" alignItems="flex-end">
            <FormControl sx={{ width: 130 }}><FormLabel>On</FormLabel>
              <Select size="sm" value={f.entity} onChange={(_, v) => setF({ ...f, entity: v ?? "lead" })} slotProps={{ button: { "data-testid": "crm-field-entity" } as never }}>
                <Option value="lead">Leads</Option><Option value="contact">Contacts</Option><Option value="company">Companies</Option><Option value="deal">Deals</Option>
              </Select></FormControl>
            <FormControl sx={{ width: 170 }}><FormLabel>Label</FormLabel><Input size="sm" value={f.label} onChange={(e) => setF({ ...f, label: e.target.value, key: slug(e.target.value) })} slotProps={{ input: { "data-testid": "crm-field-label" } }} /></FormControl>
            <FormControl sx={{ width: 130 }}><FormLabel>Type</FormLabel>
              <Select size="sm" value={f.type} onChange={(_, v) => setF({ ...f, type: v ?? "text" })} slotProps={{ button: { "data-testid": "crm-field-type" } as never }}>{TYPES.map((t) => <Option key={t} value={t}>{t}</Option>)}</Select></FormControl>
            {(f.type === "select" || f.type === "multiselect") && <FormControl sx={{ width: 200 }}><FormLabel>Options (comma)</FormLabel><Input size="sm" value={f.options} onChange={(e) => setF({ ...f, options: e.target.value })} slotProps={{ input: { "data-testid": "crm-field-options" } }} /></FormControl>}
            <Checkbox size="sm" label="Required" checked={f.required} onChange={(e) => setF({ ...f, required: e.target.checked })} />
            <Button size="sm" disabled={!f.label.trim() || !f.key} data-testid="crm-field-add" onClick={async () => {
              const body = { entity: f.entity, key: f.key, label: f.label.trim(), type: f.type, options: f.options.split(",").map((s) => s.trim()).filter(Boolean), required: f.required, position: data.fields.length };
              if (await run(() => api2.save("fields", "", body))) { setF({ ...f, label: "", key: "", options: "" }); data.reload(); }
            }}>Add field</Button>
          </Stack>
        </Sheet>
      )}
      <ErrorAlert err={err} />
    </Stack>
  );
}
