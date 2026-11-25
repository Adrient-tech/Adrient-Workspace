import { PersonSelect } from "../../components/PeoplePicker";
import { useEffect, useMemo, useState } from "react";
import {
  Box,
  Button,
  ButtonGroup,
  Chip,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Option,
  Select,
  Sheet,
  Stack,
  Table,
  Textarea,
  Typography,
} from "@mui/joy";
import { useNavigate } from "react-router-dom";
import * as api from "./api";
import { BILLING_LABEL, allStages, daysInStage, isOpen, isStalled, lastTouch, dealContacts, isCold, money, mrr, stageName, todayStr } from "./logic";
import type { Billing, Deal, DealInput, LinkedDoc, Member } from "./types";
import { ConfirmButton, ContactPicker, Data, Dialog, Empty, ErrorAlert, ago, companyName, contactName, fmtDate, useErr } from "./ui";
import { FollowupForm, ActivityForm } from "./Agenda";
import { CustomFieldInputs, CustomFilter, defsFor, matchesCustom, showValue } from "./CustomFields";
import { SavedViews } from "./SavedViews";
import { stageSlaBreach } from "./sla";
import { DealItemsEditor } from "./Products";
import { ChecklistPanel } from "./Automation";
import { Customer360Button } from "./Customer360";

const ROLES = ["champion", "decision maker", "influencer", "blocker", "user"];

interface Filter {
  q: string;
  stage: string;
  window: string; // "" | "30" | "90" | "overdue"
  sort: string;
  cf: string; // custom field key
  cfv: string; // custom field value (contains)
}
const EMPTY_FILTER: Filter = { q: "", stage: "", window: "", sort: "updated", cf: "", cfv: "" };

export function Pipeline({ data, initialDeal }: { data: Data; initialDeal: string }) {
  const [mode, setMode] = useState<"board" | "list">("board");
  const [filter, setFilter] = useState<Filter>(EMPTY_FILTER);
  const [editing, setEditing] = useState<Deal | "new" | null>(null);
  const [detail, setDetail] = useState(initialDeal);
  const [closing, setClosing] = useState<{ deal: Deal; stage: string } | null>(null);
  const [dragId, setDragId] = useState("");
  const { err, run } = useErr();
  const s = data.settings;

  useEffect(() => setDetail(initialDeal), [initialDeal]);

  const dealDefs = useMemo(() => defsFor(data.fields, "deal"), [data.fields]);
  const filtered = useMemo(() => {
    const q = filter.q.trim().toLowerCase();
    const today = todayStr();
    const horizon = (n: number) => new Date(Date.now() + n * 86400000).toISOString().slice(0, 10);
    const list = data.deals.filter((d) => {
      if (q && !`${d.title} ${companyName(data, d.company_id)} ${contactName(data, d.contact_id)}`.toLowerCase().includes(q)) return false;
      if (filter.stage && d.stage !== filter.stage) return false;
      if (filter.cf && !matchesCustom(dealDefs.find((x) => x.key === filter.cf), d.custom, filter.cfv, data.users)) return false;
      const dt = d.expected_close ?? "";
      if (filter.window === "overdue" && !(isOpen(d) && dt && dt < today)) return false;
      if ((filter.window === "30" || filter.window === "90") && !(dt && dt >= today && dt <= horizon(Number(filter.window)))) return false;
      return true;
    });
    const by: Record<string, (a: Deal, b: Deal) => number> = {
      updated: (a, b) => b.updated_at.localeCompare(a.updated_at),
      value: (a, b) => b.value - a.value,
      close: (a, b) => (a.expected_close ?? "9999").localeCompare(b.expected_close ?? "9999"),
      title: (a, b) => a.title.localeCompare(b.title),
    };
    return [...list].sort(by[filter.sort] ?? by.updated);
  }, [data, filter]);

  const move = async (d: Deal, stage: string) => {
    if (d.stage === stage) return;
    if (stage === "won" || stage === "lost") {
      setClosing({ deal: d, stage });
      return;
    }
    await run(() => api.moveDeal(d.id, stage));
    data.reload();
  };

  const openStages = s.stages;
  const wonLost = (id: string) => data.deals.filter((d) => d.stage === id);

  return (
    <Box>
      <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center" sx={{ mb: 1 }}>
        <Input placeholder="Search deals" value={filter.q} onChange={(e) => setFilter({ ...filter, q: e.target.value })} sx={{ width: 200 }} slotProps={{ input: { "aria-label": "Search deals" } }} />
        <Select size="sm" value={filter.stage} onChange={(_, v) => setFilter({ ...filter, stage: v ?? "" })} placeholder="All stages" sx={{ width: 150 }} aria-label="Stage filter">
          <Option value="">All stages</Option>
          {allStages(s).map((st) => (
            <Option key={st.id} value={st.id}>
              {st.name}
            </Option>
          ))}
        </Select>
        <Select size="sm" value={filter.window} onChange={(_, v) => setFilter({ ...filter, window: v ?? "" })} sx={{ width: 160 }} aria-label="Close window">
          <Option value="">Any close date</Option>
          <Option value="30">Closing in 30 days</Option>
          <Option value="90">Closing in 90 days</Option>
          <Option value="overdue">Overdue close date</Option>
        </Select>
        <Select size="sm" value={filter.sort} onChange={(_, v) => setFilter({ ...filter, sort: v ?? "updated" })} sx={{ width: 150 }} aria-label="Sort">
          <Option value="updated">Recently updated</Option>
          <Option value="value">Biggest first</Option>
          <Option value="close">Closing soonest</Option>
          <Option value="title">Name</Option>
        </Select>
        <CustomFilter defs={dealDefs} field={filter.cf} needle={filter.cfv} onChange={(cf, cfv) => setFilter({ ...filter, cf, cfv })} />
        <Box sx={{ flex: 1 }} />
        <ButtonGroup size="sm">
          <Button variant={mode === "board" ? "solid" : "outlined"} onClick={() => setMode("board")} data-testid="crm-mode-board">
            Board
          </Button>
          <Button variant={mode === "list" ? "solid" : "outlined"} onClick={() => setMode("list")} data-testid="crm-mode-list">
            List
          </Button>
        </ButtonGroup>
        <Button onClick={() => setEditing("new")} data-testid="crm-new-deal">
          New deal
        </Button>
      </Stack>
      <SavedViews entity="deal" getConfig={() => ({ ...filter })} onApply={(c) => setFilter({ ...EMPTY_FILTER, ...(c as Partial<Filter>) })} onClear={() => setFilter(EMPTY_FILTER)} />
      <ErrorAlert err={err} />

      {data.deals.length === 0 && <Empty>No deals yet. Create your first deal to start the pipeline.</Empty>}

      {data.deals.length > 0 && mode === "board" && (
        <Box sx={{ display: "flex", gap: 1.5, overflowX: "auto", pb: 1 }} data-testid="crm-board">
          {openStages.map((st) => {
            const col = filtered.filter((d) => d.stage === st.id);
            return (
              <Sheet
                key={st.id}
                variant="soft"
                data-testid={`crm-col-${st.id}`}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => {
                  const d = data.deals.find((x) => x.id === dragId);
                  setDragId("");
                  if (d) void move(d, st.id);
                }}
                sx={{ minWidth: 240, width: 260, flexShrink: 0, p: 1, borderRadius: "md" }}
              >
                <Stack direction="row" justifyContent="space-between" sx={{ mb: 1 }}>
                  <Typography level="title-sm">
                    {st.name} <Chip size="sm">{col.length}</Chip>
                  </Typography>
                  <Typography level="body-xs">{money(col.reduce((a, d) => a + d.value, 0), s.currency)}</Typography>
                </Stack>
                <Stack gap={1}>
                  {col.map((d) => (
                    <DealCard key={d.id} d={d} data={data} onOpen={() => setDetail(d.id)} onMove={(st2) => move(d, st2)} onDrag={() => setDragId(d.id)} />
                  ))}
                </Stack>
              </Sheet>
            );
          })}
          <Stack gap={1} sx={{ minWidth: 180 }}>
            {["won", "lost"].map((id) => (
              <Sheet
                key={id}
                variant="outlined"
                data-testid={`crm-col-${id}`}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => {
                  const d = data.deals.find((x) => x.id === dragId);
                  setDragId("");
                  if (d) void move(d, id);
                }}
                sx={{ p: 1.5, borderRadius: "md" }}
              >
                <Typography level="title-sm">{stageName(s, id)}</Typography>
                <Typography level="body-sm">
                  {wonLost(id).length} deals · {money(wonLost(id).reduce((a, d) => a + d.value, 0), s.currency)}
                </Typography>
                <Button size="sm" variant="plain" onClick={() => setFilter({ ...filter, stage: id })}>
                  Show
                </Button>
              </Sheet>
            ))}
          </Stack>
        </Box>
      )}

      {data.deals.length > 0 && mode === "list" && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table hoverRow data-testid="crm-deal-list">
            <thead>
              <tr>
                <th>Deal</th>
                <th>Company</th>
                <th>Stage</th>
                <th style={{ textAlign: "right" }}>Value</th>
                <th>Close</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((d) => (
                <tr key={d.id} onClick={() => setDetail(d.id)} style={{ cursor: "pointer" }}>
                  <td>{d.title}</td>
                  <td>{companyName(data, d.company_id)}</td>
                  <td>{stageName(s, d.stage)}</td>
                  <td style={{ textAlign: "right" }}>{money(d.value, d.currency)}</td>
                  <td>{fmtDate(d.expected_close)}</td>
                  <td>{fmtDate(d.updated_at)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
          {filtered.length === 0 && <Empty>No deals match these filters.</Empty>}
        </Sheet>
      )}

      {editing && (
        <DealDialog
          data={data}
          deal={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={(d) => {
            setEditing(null);
            data.reload();
            setDetail(d.id);
          }}
        />
      )}
      {closing && (
        <CloseDialog
          deal={closing.deal}
          stage={closing.stage}
          onClose={() => setClosing(null)}
          onDone={() => {
            setClosing(null);
            data.reload();
          }}
        />
      )}
      {detail && data.deals.find((d) => d.id === detail) && (
        <DealDetail
          data={data}
          deal={data.deals.find((d) => d.id === detail)!}
          onClose={() => setDetail("")}
          onEdit={(d) => setEditing(d)}
          onMove={(d, st) => move(d, st)}
        />
      )}
    </Box>
  );
}

function DealCard({ d, data, onOpen, onMove, onDrag }: { d: Deal; data: Data; onOpen: () => void; onMove: (stage: string) => void; onDrag: () => void }) {
  const s = data.settings;
  const stalled = isStalled(d, s.stale_days);
  const last = lastTouch(data.activities, dealContacts(d), d.id);
  const cold = isOpen(d) && isCold(last, s.cold_days);
  return (
    <Sheet variant="outlined" draggable onDragStart={onDrag} data-testid="crm-card" sx={{ p: 1, borderRadius: "sm", bgcolor: "background.surface", cursor: "grab" }}>
      <Typography level="title-sm" sx={{ cursor: "pointer" }} onClick={onOpen}>
        {d.title}
      </Typography>
      <Typography level="body-xs">{companyName(data, d.company_id) || contactName(data, d.contact_id)}</Typography>
      <Stack direction="row" gap={0.5} alignItems="center" flexWrap="wrap" sx={{ mt: 0.5 }}>
        <Typography level="body-sm" fontWeight="lg">
          {money(d.value, d.currency)}
          {d.billing !== "one_time" ? ` / ${d.billing === "monthly" ? "mo" : d.billing === "yearly" ? "yr" : "qtr"}` : ""}
        </Typography>
        {stalled && (
          <Chip size="sm" color="warning" variant="soft">
            {daysInStage(d)}d in stage
          </Chip>
        )}
        {cold && (
          <Chip size="sm" color="neutral" variant="soft">
            going cold
          </Chip>
        )}
        {isOpen(d) && stageSlaBreach(d, s) > 0 && (
          <Chip size="sm" color="danger" variant="soft" data-testid="crm-stage-sla">
            over {stageSlaBreach(d, s)}d limit
          </Chip>
        )}
      </Stack>
      <Select size="sm" variant="plain" value={d.stage} onChange={(_, v) => v && onMove(v)} sx={{ mt: 0.5, ml: -1 }} aria-label={`Move ${d.title}`}>
        {allStages(s).map((st) => (
          <Option key={st.id} value={st.id}>
            {st.name}
          </Option>
        ))}
      </Select>
    </Sheet>
  );
}

function CloseDialog({ deal, stage, onClose, onDone }: { deal: Deal; stage: string; onClose: () => void; onDone: () => void }) {
  const [reason, setReason] = useState("");
  const { err, run } = useErr();
  return (
    <Dialog title={stage === "won" ? `Mark "${deal.title}" as won` : `Mark "${deal.title}" as lost`} onClose={onClose} width={440}>
      <FormControl>
        <FormLabel>{stage === "won" ? "Why did it close?" : "Why was it lost?"} (optional)</FormLabel>
        <Input value={reason} autoFocus onChange={(e) => setReason(e.target.value)} placeholder={stage === "won" ? "Price, relationship, timing…" : "Budget, competitor, no decision…"} data-testid="crm-reason" />
      </FormControl>
      <ErrorAlert err={err} />
      <Stack direction="row" justifyContent="flex-end" gap={1}>
        <Button
          data-testid="crm-close-confirm"
          onClick={async () => {
            if (await run(() => api.moveDeal(deal.id, stage, reason.trim()))) onDone();
          }}
        >
          Confirm
        </Button>
      </Stack>
    </Dialog>
  );
}

function DealDialog({ data, deal, onClose, onSaved }: { data: Data; deal: Deal | null; onClose: () => void; onSaved: (d: Deal) => void }) {
  const s = data.settings;
  const [f, setF] = useState<DealInput>({
    title: deal?.title ?? "",
    company_id: deal?.company_id ?? "",
    contact_id: deal?.contact_id ?? "",
    stage: deal?.stage ?? s.stages[0].id,
    value: deal?.value ?? 0,
    currency: deal?.currency ?? s.currency,
    billing: deal?.billing ?? "one_time",
    expected_close: deal?.expected_close ?? "",
    source: deal?.source ?? "",
    notes: deal?.notes ?? "",
    committee: deal?.committee ?? [],
    owner_id: deal?.owner_id ?? "",
    custom: deal?.custom ?? {},
    items: deal?.items ?? [],
  });
  const { err, run } = useErr();
  const set = <K extends keyof DealInput>(k: K, v: DealInput[K]) => setF((p) => ({ ...p, [k]: v }));
  const m = mrr({ value: f.value, billing: f.billing });
  return (
    <Dialog title={deal ? "Edit deal" : "New deal"} onClose={onClose}>
      <Stack gap={1.5}>
        <FormControl required>
          <FormLabel>Deal name</FormLabel>
          <Input value={f.title} autoFocus onChange={(e) => set("title", e.target.value)} data-testid="crm-deal-title" />
        </FormControl>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Company</FormLabel>
            <Select value={f.company_id} onChange={(_, v) => set("company_id", v ?? "")} aria-label="Company">
              <Option value="">None</Option>
              {data.companies.map((c) => (
                <Option key={c.id} value={c.id}>
                  {c.name}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Primary contact</FormLabel>
            <ContactPicker contacts={data.contacts} value={f.contact_id} onChange={(id) => set("contact_id", id)} testid="crm-deal-contact" />
          </FormControl>
        </Stack>
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Value ({f.currency})</FormLabel>
            <Input type="number" value={f.value} onChange={(e) => set("value", Number(e.target.value))} slotProps={{ input: { min: 0, "data-testid": "crm-deal-value" } }} />
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Billing</FormLabel>
            <Select value={f.billing} onChange={(_, v) => set("billing", (v ?? "one_time") as Billing)} aria-label="Billing">
              {Object.entries(BILLING_LABEL).map(([k, l]) => (
                <Option key={k} value={k}>
                  {l}
                </Option>
              ))}
            </Select>
          </FormControl>
        </Stack>
        {f.billing !== "one_time" && <Typography level="body-xs">Adds {money(m, f.currency)} MRR when won.</Typography>}
        <Stack direction="row" gap={1}>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Stage</FormLabel>
            <Select value={f.stage} onChange={(_, v) => set("stage", v ?? f.stage)} aria-label="Stage">
              {allStages(s).map((st) => (
                <Option key={st.id} value={st.id}>
                  {st.name}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl sx={{ flex: 1 }}>
            <FormLabel>Expected close</FormLabel>
            <Input type="date" value={f.expected_close} onChange={(e) => set("expected_close", e.target.value)} />
          </FormControl>
        </Stack>
        <FormControl>
          <FormLabel>Lead source</FormLabel>
          <Input value={f.source} onChange={(e) => set("source", e.target.value)} placeholder="Referral, website, event…" />
        </FormControl>
        <FormControl>
          <FormLabel>Buying committee</FormLabel>
          <Stack gap={0.5}>
            {f.committee.map((c, i) => (
              <Stack key={i} direction="row" gap={0.5}>
                <Box sx={{ flex: 1 }}>
                  <ContactPicker contacts={data.contacts} value={c.contact_id} onChange={(id) => set("committee", f.committee.map((x, j) => (j === i ? { ...x, contact_id: id } : x)))} />
                </Box>
                <Select size="sm" value={c.role} onChange={(_, v) => set("committee", f.committee.map((x, j) => (j === i ? { ...x, role: v ?? "" } : x)))} sx={{ width: 150 }} aria-label="Role">
                  <Option value="">No role</Option>
                  {ROLES.map((r) => (
                    <Option key={r} value={r}>
                      {r}
                    </Option>
                  ))}
                </Select>
                <IconButton size="sm" aria-label="Remove member" onClick={() => set("committee", f.committee.filter((_, j) => j !== i))}>
                  ×
                </IconButton>
              </Stack>
            ))}
            <Button size="sm" variant="soft" sx={{ alignSelf: "flex-start" }} onClick={() => set("committee", [...f.committee, { contact_id: "", role: "" } as Member])}>
              Add person
            </Button>
          </Stack>
        </FormControl>
        <FormControl>
          <FormLabel>Owner</FormLabel>
          <PersonSelect value={f.owner_id ?? ""} known={data.users} onChange={(id) => set("owner_id", id)} placeholder={deal ? "Keep current owner" : "Me"} ariaLabel="Owner" />
        </FormControl>
        <CustomFieldInputs defs={defsFor(data.fields, "deal")} value={f.custom ?? {}} onChange={(v) => set("custom", v)} users={data.users} />
        <DealItemsEditor items={f.items ?? []} currency={f.currency} onChange={(items) => setF((p) => ({ ...p, items, value: items.length ? items.reduce((a, i) => a + Math.round(i.qty * i.unit_price * (1 - i.discount_pct / 100) * 100) / 100, 0) : p.value }))} />
        <FormControl>
          <FormLabel>Notes</FormLabel>
          <Textarea minRows={2} value={f.notes} onChange={(e) => set("notes", e.target.value)} />
        </FormControl>
        <ErrorAlert err={err} />
        <Stack direction="row" justifyContent="flex-end" gap={1}>
          <Button variant="plain" onClick={onClose}>
            Cancel
          </Button>
          <Button
            data-testid="crm-deal-save"
            disabled={!f.title.trim()}
            onClick={async () => {
              const r = await run(() => api.saveDeal(deal?.id ?? "", { ...f, committee: f.committee.filter((c) => c.contact_id) }));
              if (r) onSaved(r);
            }}
          >
            Save
          </Button>
        </Stack>
      </Stack>
    </Dialog>
  );
}

function DealDetail({ data, deal, onClose, onEdit, onMove }: { data: Data; deal: Deal; onClose: () => void; onEdit: (d: Deal) => void; onMove: (d: Deal, stage: string) => void }) {
  const s = data.settings;
  const nav = useNavigate();
  const { err, run } = useErr();
  const [docs, setDocs] = useState<LinkedDoc[]>([]);
  const [adding, setAdding] = useState<"" | "followup" | "activity">("");
  useEffect(() => {
    let live = true;
    void api.listLinkedDocs(deal.contact_id ?? "").then((d) => live && setDocs(d));
    return () => {
      live = false;
    };
  }, [deal.contact_id, deal.id]);

  const hist = data.history.filter((h) => h.deal_id === deal.id);
  const acts = data.activities.filter((a) => a.deal_id === deal.id || (a.contact_id && dealContacts(deal).includes(a.contact_id)));
  const fups = data.followups.filter((f) => f.deal_id === deal.id);
  const people = [deal.contact_id, ...deal.committee.map((m) => m.contact_id)].filter((x, i, a): x is string => !!x && a.indexOf(x) === i);

  const [links, setLinks] = useState<{ app: string; label: string; href: string }[]>([]);
  const [handoffMsg, setHandoffMsg] = useState("");
  useEffect(() => {
    let live = true;
    void api.dealLinks(deal.id).then((l) => live && setLinks(l)).catch(() => undefined);
    return () => {
      live = false;
    };
  }, [deal.id, handoffMsg]);
  const estimate = async () => {
    const r = await run(() => api.dealHandoff(deal.id, false));
    if (r) nav(`/invoicing?doc=${r.estimate_id}`);
  };
  const project = async () => {
    const r = await run(() => api.dealHandoff(deal.id, true));
    if (r) setHandoffMsg(r.project_id ? "Delivery project ready in Projects." : r.notes.join(" "));
  };

  return (
    <Dialog title={deal.title} onClose={onClose} width={760}>
      <Stack gap={1.5} data-testid="crm-deal-detail">
        <Stack direction="row" gap={1} flexWrap="wrap" alignItems="center">
          <Select size="sm" value={deal.stage} onChange={(_, v) => v && onMove(deal, v)} sx={{ width: 160 }} aria-label="Deal stage">
            {allStages(s).map((st) => (
              <Option key={st.id} value={st.id}>
                {st.name}
              </Option>
            ))}
          </Select>
          <Typography level="title-lg">{money(deal.value, deal.currency)}</Typography>
          {deal.billing !== "one_time" && (
            <Chip variant="soft" color="success">
              {BILLING_LABEL[deal.billing]} · {money(mrr(deal), deal.currency)} MRR
            </Chip>
          )}
          <Box sx={{ flex: 1 }} />
          <Button size="sm" variant="outlined" onClick={() => onEdit(deal)} data-testid="crm-deal-edit">
            Edit
          </Button>
          {deal.company_id && <Customer360Button kind="company" id={deal.company_id} />}
          <Button size="sm" variant="outlined" onClick={estimate} data-testid="crm-create-estimate">
            {links.some((l) => l.app === "invoicing") ? "Open estimate" : "Create estimate"}
          </Button>
          {deal.stage === "won" && !links.some((l) => l.app === "projects") && (
            <Button size="sm" variant="outlined" onClick={project} data-testid="crm-create-project">
              Create project
            </Button>
          )}
          <ConfirmButton
            label="Delete"
            testid="crm-deal-delete"
            onConfirm={async () => {
              if (await run(() => api.deleteDeal(deal.id))) {
                onClose();
                data.reload();
              }
            }}
          />
        </Stack>
        <ErrorAlert err={err} />
        <Typography level="body-sm">
          {companyName(data, deal.company_id) || "No company"} · {contactName(data, deal.contact_id) || "No contact"} · closes {fmtDate(deal.expected_close) || "no date"}
          {deal.source ? ` · ${deal.source}` : ""}
        </Typography>
        {isOpen(deal) && (
          <Typography level="body-sm" data-testid="crm-in-stage">
            In {stageName(s, deal.stage)} for {daysInStage(deal)} days · last touch {ago(lastTouch(data.activities, dealContacts(deal), deal.id))}
          </Typography>
        )}
        {!isOpen(deal) && deal.close_reason && <Typography level="body-sm">Reason: {deal.close_reason}</Typography>}
        {deal.notes && <Typography level="body-sm">{deal.notes}</Typography>}

        {defsFor(data.fields, "deal").some((d) => deal.custom?.[d.key] !== undefined) && (
          <Typography level="body-sm" data-testid="crm-deal-custom">
            {defsFor(data.fields, "deal").filter((d) => deal.custom?.[d.key] !== undefined).map((d) => `${d.label}: ${showValue(d, deal.custom?.[d.key], data.users)}`).join(" · ")}
          </Typography>
        )}
        {(deal.items ?? []).length > 0 && (
          <Typography level="body-sm" data-testid="crm-deal-items">
            Products: {(deal.items ?? []).map((i) => `${i.qty} x ${i.name}`).join(", ")}
          </Typography>
        )}
        <ChecklistPanel dealId={deal.id} stage={deal.stage} stageName={stageName(s, deal.stage)} onChanged={data.reload} />

        <Typography level="title-sm">Buying committee</Typography>
        {people.length === 0 && <Typography level="body-sm">No people linked yet. Edit the deal to add them.</Typography>}
        {people.map((id) => (
          <Typography key={id} level="body-sm">
            {contactName(data, id)}
            {deal.committee.find((m) => m.contact_id === id)?.role ? ` · ${deal.committee.find((m) => m.contact_id === id)!.role}` : id === deal.contact_id ? " · primary" : ""} · last touch {ago(lastTouch(data.activities, [id]))}
          </Typography>
        ))}

        <Typography level="title-sm">Stage history</Typography>
        <Typography level="body-sm" data-testid="crm-history">
          {hist.map((h) => `${stageName(s, h.stage)} (${fmtDate(h.entered_at)})`).join(" → ") || "No history"}
        </Typography>

        {(links.length > 0 || handoffMsg) && (
          <Stack direction="row" gap={0.75} flexWrap="wrap" alignItems="center" data-testid="crm-deal-links">
            {links.map((l) => (
              <Button key={l.href} size="sm" variant="soft" component="a" href={l.href}>
                {l.label}
              </Button>
            ))}
            {handoffMsg && <Typography level="body-xs">{handoffMsg}</Typography>}
          </Stack>
        )}
        <Typography level="title-sm">Linked estimates and invoices</Typography>
        {docs.length === 0 && <Typography level="body-sm">None for this contact yet.</Typography>}
        {docs.map((d) => (
          <Typography key={d.id} level="body-sm">
            <a href={`/invoicing?doc=${d.id}`}>{d.number}</a> · {d.kind} · {d.display_status} · {money(d.total, d.currency)}
          </Typography>
        ))}

        <Stack direction="row" alignItems="center" gap={1}>
          <Typography level="title-sm" sx={{ flex: 1 }}>
            Follow-ups and activity
          </Typography>
          <Button size="sm" variant="soft" onClick={() => setAdding("followup")} data-testid="crm-add-followup">
            Schedule follow-up
          </Button>
          <Button size="sm" variant="soft" onClick={() => setAdding("activity")} data-testid="crm-add-activity">
            Log activity
          </Button>
        </Stack>
        {fups.map((f) => (
          <Typography key={f.id} level="body-sm" sx={{ textDecoration: f.done ? "line-through" : undefined }}>
            Follow-up: {f.title} · {fmtDate(f.due_date)} {f.due_time}
            {f.calendar_event_id ? " · in Calendar" : ""}
          </Typography>
        ))}
        {acts.slice(0, 10).map((a) => (
          <Typography key={a.id} level="body-sm">
            {a.kind} · {fmtDate(a.occurred_at)} · {a.summary}
          </Typography>
        ))}
      </Stack>
      {adding === "followup" && (
        <FollowupForm
          data={data}
          dealId={deal.id}
          contactId={deal.contact_id ?? ""}
          onClose={() => setAdding("")}
          onSaved={() => {
            setAdding("");
            data.reload();
          }}
        />
      )}
      {adding === "activity" && (
        <ActivityForm
          data={data}
          dealId={deal.id}
          contactId={deal.contact_id ?? ""}
          onClose={() => setAdding("")}
          onSaved={() => {
            setAdding("");
            data.reload();
          }}
        />
      )}
    </Dialog>
  );
}
