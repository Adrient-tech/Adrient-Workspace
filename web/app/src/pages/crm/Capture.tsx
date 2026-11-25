import { PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, FormControl, FormLabel, Input, Option, Select, Sheet, Stack, Tab, TabList, TabPanel, Table, Tabs, Textarea, Typography } from "@mui/joy";
import * as api2 from "./api2";
import { listForms } from "../forms/api";
import type { Form } from "../forms/types";
import { defsFor } from "./CustomFields";
import { ConfirmButton, Dialog, Empty, ErrorAlert, useErr } from "./ui";
import type { Data } from "./ui";

interface FormField {
  key: string;
  label: string;
  required: boolean;
}
interface LeadForm {
  id: string;
  name: string;
  token: string;
  enabled: boolean;
  fields: FormField[];
  source: string;
  owner_ids: string[];
  thanks: string;
}
interface FormLink {
  id: string;
  form_id: string;
  mapping: Record<string, string>;
  source: string;
  owner_ids: string[];
  enabled: boolean;
}
interface Template {
  id: string;
  name: string;
  subject: string;
  body: string;
}

const BASE_FIELDS: [string, string][] = [["name", "Name"], ["email", "Email"], ["phone", "Phone"], ["company", "Company"], ["message", "Message"]];

export function Capture({ data }: { data: Data }) {
  return (
    <Box>
      <Typography level="h4" sx={{ mb: 1 }}>
        Lead capture and email
      </Typography>
      {!data.me.is_admin && <Typography level="body-sm" sx={{ mb: 1 }}>Only a CRM administrator can change these settings. Mail logging below is yours.</Typography>}
      <Tabs defaultValue="forms" sx={{ bgcolor: "transparent" }}>
        <TabList>
          <Tab value="forms" data-testid="crm-cap-tab-forms">Web forms</Tab>
          <Tab value="links" data-testid="crm-cap-tab-links">Forms app</Tab>
          <Tab value="templates" data-testid="crm-cap-tab-templates">Email templates</Tab>
          <Tab value="mail" data-testid="crm-cap-tab-mail">Mail logging</Tab>
        </TabList>
        <TabPanel value="forms"><LeadForms data={data} /></TabPanel>
        <TabPanel value="links"><FormLinks data={data} /></TabPanel>
        <TabPanel value="templates"><Templates data={data} /></TabPanel>
        <TabPanel value="mail"><MailLogging data={data} /></TabPanel>
      </Tabs>
    </Box>
  );
}

export const embedSnippet = (origin: string, token: string) =>
  `<iframe src="${origin}/api/v1/public/crm/lead/${token}" title="Contact us" style="border:0;width:100%;max-width:480px;height:520px" loading="lazy"></iframe>`;

function LeadForms({ data }: { data: Data }) {
  const [forms, setForms] = useState<LeadForm[]>([]);
  const [editing, setEditing] = useState<LeadForm | "new" | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<LeadForm>("lead-forms", "forms")).then((l) => l && setForms(l)); }, [run]);
  useEffect(load, [load]);
  return (
    <Stack gap={1.5}>
      <Stack direction="row" alignItems="center" gap={1}>
        <Typography level="body-sm" sx={{ flex: 1 }}>A public form anyone can fill in. Each submission becomes a lead (an existing lead with the same email is reused) and runs your automation rules.</Typography>
        {data.me.is_admin && <Button size="sm" onClick={() => setEditing("new")} data-testid="crm-new-leadform">New web form</Button>}
      </Stack>
      <ErrorAlert err={err} />
      {forms.length === 0 ? <Empty>No web forms yet.</Empty> : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="crm-leadforms">
            <thead><tr><th>Name</th><th>Fields</th><th>Source</th><th>Status</th></tr></thead>
            <tbody>
              {forms.map((f) => (
                <tr key={f.id} onClick={() => setEditing(f)} style={{ cursor: "pointer" }}>
                  <td>{f.name}</td><td>{f.fields.map((x) => x.key.replace("custom:", "")).join(", ")}</td><td>{f.source}</td><td>{f.enabled ? "Live" : "Off"}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {editing && <LeadFormDialog data={data} form={editing === "new" ? null : editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
    </Stack>
  );
}

function LeadFormDialog({ data, form, onClose, onSaved }: { data: Data; form: LeadForm | null; onClose: () => void; onSaved: () => void }) {
  const customDefs = defsFor(data.fields, "person");
  const all: [string, string][] = [...BASE_FIELDS, ...customDefs.map((d) => [`custom:${d.key}`, d.label] as [string, string])];
  const [f, setF] = useState<LeadForm>(
    form ?? { id: "", name: "", token: "", enabled: true, fields: [{ key: "name", label: "Name", required: true }, { key: "email", label: "Email", required: true }, { key: "message", label: "Message", required: false }], source: "web form", owner_ids: [], thanks: "Thanks, we will be in touch." },
  );
  const { err, run } = useErr();
  const has = (k: string) => f.fields.find((x) => x.key === k);
  const toggle = (k: string, label: string, on: boolean) => setF({ ...f, fields: on ? [...f.fields, { key: k, label, required: false }] : f.fields.filter((x) => x.key !== k) });
  const snippet = f.token ? embedSnippet(window.location.origin, f.token) : "";
  return (
    <Dialog title={form ? "Edit web form" : "New web form"} onClose={onClose} width={640}>
      <Stack gap={1.5}>
        <FormControl required><FormLabel>Name</FormLabel><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "crm-leadform-name" } }} /></FormControl>
        <FormControl>
          <FormLabel>Fields</FormLabel>
          <Stack gap={0.5}>
            {all.map(([k, l]) => (
              <Stack key={k} direction="row" gap={1} alignItems="center">
                <Checkbox size="sm" label={l} checked={!!has(k)} onChange={(e) => toggle(k, l, e.target.checked)} data-testid={`crm-leadform-field-${k}`} sx={{ width: 160 }} />
                {has(k) && <Checkbox size="sm" label="Required" checked={has(k)!.required} onChange={(e) => setF({ ...f, fields: f.fields.map((x) => (x.key === k ? { ...x, required: e.target.checked } : x)) })} />}
              </Stack>
            ))}
          </Stack>
        </FormControl>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}><FormLabel>Lead source</FormLabel><Input value={f.source} onChange={(e) => setF({ ...f, source: e.target.value })} /></FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Owners (round robin)</FormLabel>
            <PeopleSelect value={f.owner_ids} known={data.users} placeholder="Me" onChange={(ids) => setF({ ...f, owner_ids: ids })} ariaLabel="Owners" />
          </FormControl>
        </Stack>
        <FormControl><FormLabel>Thank-you message</FormLabel><Input value={f.thanks} onChange={(e) => setF({ ...f, thanks: e.target.value })} /></FormControl>
        {form && <Checkbox label="Form is live" checked={f.enabled} onChange={(e) => setF({ ...f, enabled: e.target.checked })} />}
        {snippet && (
          <FormControl>
            <FormLabel>Embed on your website</FormLabel>
            <Textarea minRows={3} readOnly value={snippet} slotProps={{ textarea: { "data-testid": "crm-leadform-snippet" } }} />
            <Typography level="body-xs">Public link: {window.location.origin}/api/v1/public/crm/lead/{f.token}</Typography>
          </FormControl>
        )}
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="space-between">
          {form ? <ConfirmButton label="Delete" onConfirm={async () => { if (await run(() => api2.remove("lead-forms", form.id))) onSaved(); }} /> : <span />}
          <Button disabled={!f.name.trim()} data-testid="crm-leadform-save" onClick={async () => { if (await run(() => api2.save("lead-forms", form?.id ?? "", f))) onSaved(); }}>Save</Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}

function FormLinks({ data }: { data: Data }) {
  const [links, setLinks] = useState<FormLink[]>([]);
  const [forms, setForms] = useState<Form[]>([]);
  const [formId, setFormId] = useState("");
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [source, setSource] = useState("forms");
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<FormLink>("form-links", "links")).then((l) => l && setLinks(l)); }, [run]);
  useEffect(load, [load]);
  useEffect(() => { void listForms().then(setForms).catch(() => setForms([])); }, []);
  const form = forms.find((f) => f.id === formId);
  const opts: [string, string][] = [["", "Not used"], ...BASE_FIELDS, ...defsFor(data.fields, "person").map((d) => [`custom:${d.key}`, d.label] as [string, string])];
  const pick = (id: string) => {
    setFormId(id);
    const l = links.find((x) => x.form_id === id);
    setMapping(l?.mapping ?? {});
    setSource(l?.source ?? "forms");
  };
  return (
    <Stack gap={1.5} data-testid="crm-formlinks">
      <Typography level="body-sm">Link a Forms-app form to the CRM: every response creates a lead from the questions you map. People who are signed in to Adrient fill the form as usual.</Typography>
      {links.map((l) => (
        <Stack key={l.id} direction="row" gap={1} alignItems="center">
          <Typography level="body-sm" sx={{ flex: 1 }}>{forms.find((f) => f.id === l.form_id)?.title ?? l.form_id} · {Object.keys(l.mapping).length} questions mapped · {l.enabled ? "on" : "off"}</Typography>
          {data.me.is_admin && <Button size="sm" variant="plain" onClick={() => pick(l.form_id)}>Edit</Button>}
          {data.me.is_admin && <ConfirmButton label="Unlink" onConfirm={async () => { if (await run(() => api2.remove("form-links", l.id))) load(); }} />}
        </Stack>
      ))}
      {data.me.is_admin && (
        <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md" }}>
          <Stack gap={1}>
            <FormControl>
              <FormLabel>Form</FormLabel>
              <Select value={formId} onChange={(_, v) => pick(v ?? "")} placeholder="Choose a form" slotProps={{ button: { "data-testid": "crm-link-form" } as never }}>
                {forms.map((f) => <Option key={f.id} value={f.id}>{f.title || "Untitled form"}</Option>)}
              </Select>
              {forms.length === 0 && <Typography level="body-xs">No forms found. Create one in the Forms app first.</Typography>}
            </FormControl>
            {form && form.questions.filter((q) => !q.is_section && q.type !== "file_upload").map((q) => (
              <Stack key={q.id} direction="row" gap={1} alignItems="center">
                <Typography level="body-sm" sx={{ flex: 1 }}>{q.title}</Typography>
                <Select size="sm" sx={{ width: 180 }} value={mapping[q.id] ?? ""} onChange={(_, v) => setMapping((m) => { const n = { ...m }; if (v) n[q.id] = v; else delete n[q.id]; return n; })} aria-label={`Map ${q.title}`} slotProps={{ button: { "data-testid": `crm-link-q-${q.title}` } as never }}>
                  {opts.map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
                </Select>
              </Stack>
            ))}
            {form && <FormControl><FormLabel>Lead source</FormLabel><Input size="sm" value={source} onChange={(e) => setSource(e.target.value)} /></FormControl>}
            <ErrorAlert err={err} />
            {form && <Button size="sm" data-testid="crm-link-save" onClick={async () => { if (await run(() => api2.save("form-links", "", { form_id: form.id, mapping, source, owner_ids: [], enabled: true }))) { load(); setFormId(""); } }}>Save link</Button>}
          </Stack>
        </Sheet>
      )}
    </Stack>
  );
}

function Templates({ data }: { data: Data }) {
  const [list, setList] = useState<Template[]>([]);
  const [editing, setEditing] = useState<Template | "new" | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => { void run(() => api2.list<Template>("templates", "templates")).then((l) => l && setList(l)); }, [run]);
  useEffect(load, [load]);
  return (
    <Stack gap={1.5} data-testid="crm-templates">
      <Stack direction="row" alignItems="center" gap={1}>
        <Typography level="body-sm" sx={{ flex: 1 }}>Templates can use {"{{person.name}}"}, {"{{person.email}}"}, {"{{company.name}}"}, {"{{deal.title}}"}, {"{{deal.stage}}"}, {"{{deal.value}}"} and {"{{owner.name}}"}. Mails carry no tracking pixel or link tracking.</Typography>
        {data.me.is_admin && <Button size="sm" onClick={() => setEditing("new")} data-testid="crm-new-template">New template</Button>}
      </Stack>
      <ErrorAlert err={err} />
      {list.length === 0 ? <Empty>No templates yet.</Empty> : list.map((t) => (
        <Stack key={t.id} direction="row" gap={1} alignItems="center"><Typography level="body-sm" sx={{ flex: 1 }}>{t.name} · {t.subject}</Typography>{data.me.is_admin && <Button size="sm" variant="plain" onClick={() => setEditing(t)}>Edit</Button>}</Stack>
      ))}
      {editing && <TemplateDialog tpl={editing === "new" ? null : editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); load(); }} />}
    </Stack>
  );
}

function TemplateDialog({ tpl, onClose, onSaved }: { tpl: Template | null; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ name: tpl?.name ?? "", subject: tpl?.subject ?? "", body: tpl?.body ?? "" });
  const { err, run } = useErr();
  return (
    <Dialog title={tpl ? "Edit template" : "New template"} onClose={onClose} width={560}>
      <Stack gap={1.5}>
        <FormControl required><FormLabel>Name</FormLabel><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "crm-template-name" } }} /></FormControl>
        <FormControl required><FormLabel>Subject</FormLabel><Input value={f.subject} onChange={(e) => setF({ ...f, subject: e.target.value })} slotProps={{ input: { "data-testid": "crm-template-subject" } }} /></FormControl>
        <FormControl><FormLabel>Body</FormLabel><Textarea minRows={6} value={f.body} onChange={(e) => setF({ ...f, body: e.target.value })} slotProps={{ textarea: { "data-testid": "crm-template-body" } }} /></FormControl>
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="space-between">
          {tpl ? <ConfirmButton label="Delete" onConfirm={async () => { if (await run(() => api2.remove("templates", tpl.id))) onSaved(); }} /> : <span />}
          <Button disabled={!f.name.trim() || !f.subject.trim()} data-testid="crm-template-save" onClick={async () => { if (await run(() => api2.save("templates", tpl?.id ?? "", f))) onSaved(); }}>Save</Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}

function MailLogging({ data }: { data: Data }) {
  const [auto, setAuto] = useState(data.me.auto_log_mail);
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();
  return (
    <Stack gap={1.5} data-testid="crm-maillog">
      <Typography level="body-sm">
        The CRM can note which of your emails (last 90 days, inbox and sent) were to or from a lead or contact, matched by email address. Only the subject, a short preview and the conversation link are stored. The message body is never copied, and only your own mailbox is read.
      </Typography>
      <Stack direction="row" gap={1} alignItems="center">
        <Button size="sm" data-testid="crm-sync-mail" onClick={async () => { const r = await run(() => api2.post<{ logged: number }>("mail/sync")); if (r) { setMsg(`Logged ${r.logged} new email(s).`); data.reload(); } }}>Log my mail now</Button>
        {msg && <Typography level="body-sm" color="success" data-testid="crm-sync-msg">{msg}</Typography>}
      </Stack>
      <Checkbox label="Keep logging my mail automatically (every few minutes)" checked={auto} data-testid="crm-auto-log" onChange={async (e) => { const v = e.target.checked; if (await run(() => api2.put("me", { auto_log_mail: v }))) setAuto(v); }} />
      <Typography level="body-xs">Email open tracking is not offered: it would need tracking pixels, which this workspace does not add.</Typography>
      <ErrorAlert err={err} />
    </Stack>
  );
}
