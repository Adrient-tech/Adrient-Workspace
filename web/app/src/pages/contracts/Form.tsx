import { useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, DialogContent, DialogTitle, FormControl, FormLabel, Input, List, ListItem, ListItemButton, Modal, ModalClose, ModalDialog, Option, Select, Textarea, Typography } from "@mui/joy";
import { contracts, type Contract, type ContractInput } from "./api";
import { TYPE_LABEL } from "./logic";

const NEW: ContractInput = { title: "", type: "service", status: "active", party_name: "", value: 0, currency: "INR", billing: "monthly", auto_renew: false, term_months: 12, notice_days: 30, access: "org", remind: true, tags: [] };
const LB = { listbox: { sx: { zIndex: 2000 } } };

export function ContractForm({ initial, onClose, onSaved }: { initial?: Contract; onClose: () => void; onSaved: (c: Contract) => void }) {
  const [f, setF] = useState<ContractInput>(initial ? { ...initial } : NEW);
  const [partyQ, setPartyQ] = useState("");
  const [parties, setParties] = useState<{ id: string; name: string; company: string }[]>([]);
  const [tags, setTags] = useState((initial?.tags ?? []).join(", "));
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const set = <K extends keyof ContractInput>(k: K, v: ContractInput[K]) => setF((x) => ({ ...x, [k]: v }));
  useEffect(() => {
    const t = setTimeout(() => contracts.parties(partyQ).then(setParties).catch(() => setParties([])), 200);
    return () => clearTimeout(t);
  }, [partyQ]);
  const save = async () => {
    setBusy(true);
    setErr("");
    try {
      const body = { ...f, tags: tags.split(",").map((s) => s.trim()).filter(Boolean), value: Number(f.value) || 0, term_months: Number(f.term_months) || 12, notice_days: Number(f.notice_days) || 0 };
      onSaved(initial ? await contracts.update(initial.id, body) : await contracts.create(body));
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog
        sx={{ width: 640, maxWidth: "100vw", maxHeight: "96vh", overflow: "auto", "@media (max-width: 600px)": { width: "100vw", height: "100vh", maxHeight: "100vh", borderRadius: 0 } }}
        role="dialog" aria-label={initial ? "Edit contract" : "New contract"}
      >
        <ModalClose />
        <DialogTitle>{initial ? "Edit contract" : "New contract"}</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1.5 }}>
            <FormControl sx={{ gridColumn: "1 / -1" }}>
              <FormLabel>Title</FormLabel>
              <Input autoFocus value={f.title ?? ""} onChange={(e) => set("title", e.target.value)} data-testid="c-title" />
            </FormControl>
            <FormControl>
              <FormLabel>Type</FormLabel>
              <Select value={f.type ?? "other"} onChange={(_, v) => v && set("type", v)} slotProps={LB}>
                {Object.entries(TYPE_LABEL).map(([k, l]) => <Option key={k} value={k}>{l}</Option>)}
              </Select>
            </FormControl>
            <FormControl>
              <FormLabel>Status</FormLabel>
              <Select value={f.status ?? "active"} onChange={(_, v) => v && set("status", v as Contract["status"])} slotProps={LB}>
                <Option value="draft">Draft</Option><Option value="active">Active</Option><Option value="terminated">Terminated</Option>
              </Select>
            </FormControl>
            <FormControl sx={{ gridColumn: "1 / -1" }}>
              <FormLabel>Counterparty</FormLabel>
              <Input value={f.party_name ?? ""} onChange={(e) => set("party_name", e.target.value)} placeholder="Company or person name" />
              <Input size="sm" sx={{ mt: 0.5 }} value={partyQ} onChange={(e) => setPartyQ(e.target.value)} placeholder="…or link a contact from Contacts" slotProps={{ input: { "aria-label": "Search contacts" } }} />
              {f.contact_id && <Chip size="sm" sx={{ mt: 0.5, alignSelf: "flex-start" }} onClick={() => set("contact_id", "")}>Linked contact ✕</Chip>}
              {partyQ && parties.length > 0 && (
                <List size="sm" sx={{ maxHeight: 130, overflow: "auto", border: "1px solid", borderColor: "divider", borderRadius: "sm" }}>
                  {parties.map((p) => (
                    <ListItem key={p.id}>
                      <ListItemButton onClick={() => { setF((x) => ({ ...x, contact_id: p.id, party_name: x.party_name || p.company || p.name })); setPartyQ(""); }}>
                        {p.name}{p.company ? ` · ${p.company}` : ""}
                      </ListItemButton>
                    </ListItem>
                  ))}
                </List>
              )}
            </FormControl>
            <FormControl>
              <FormLabel>Value</FormLabel>
              <Input type="number" value={f.value ?? 0} onChange={(e) => set("value", Number(e.target.value))} slotProps={{ input: { step: "0.01", min: 0 } }} />
            </FormControl>
            <Box sx={{ display: "flex", gap: 1 }}>
              <FormControl sx={{ width: 90 }}>
                <FormLabel>Currency</FormLabel>
                <Input value={f.currency ?? "INR"} onChange={(e) => set("currency", e.target.value.toUpperCase().slice(0, 3))} />
              </FormControl>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Billing</FormLabel>
                <Select value={f.billing ?? "one-time"} onChange={(_, v) => v && set("billing", v)} slotProps={LB}>
                  {["one-time", "monthly", "quarterly", "annual"].map((b) => <Option key={b} value={b}>{b}</Option>)}
                </Select>
              </FormControl>
            </Box>
            <FormControl><FormLabel>Start date</FormLabel><Input type="date" value={f.start_date ?? ""} onChange={(e) => set("start_date", e.target.value)} /></FormControl>
            <FormControl><FormLabel>End date</FormLabel><Input type="date" value={f.end_date ?? ""} onChange={(e) => set("end_date", e.target.value)} data-testid="c-end" /></FormControl>
            <Box sx={{ gridColumn: "1 / -1" }}>
              <Checkbox label="Renews automatically at the end of each term" checked={!!f.auto_renew} onChange={(e) => set("auto_renew", e.target.checked)} />
            </Box>
            {f.auto_renew && (
              <FormControl>
                <FormLabel>Renewal term (months)</FormLabel>
                <Input type="number" value={f.term_months ?? 12} onChange={(e) => set("term_months", Number(e.target.value))} slotProps={{ input: { min: 1 } }} />
              </FormControl>
            )}
            <FormControl>
              <FormLabel>Notice period (days before the end)</FormLabel>
              <Input type="number" value={f.notice_days ?? 0} onChange={(e) => set("notice_days", Number(e.target.value))} slotProps={{ input: { min: 0 } }} />
            </FormControl>
            <FormControl>
              <FormLabel>Who can see it</FormLabel>
              <Select value={f.access ?? "org"} onChange={(_, v) => v && set("access", v as Contract["access"])} slotProps={LB}>
                <Option value="org">Everyone with the Contracts app</Option><Option value="restricted">Only me and admins</Option>
              </Select>
            </FormControl>
            <FormControl><FormLabel>Tags (comma separated)</FormLabel><Input value={tags} onChange={(e) => setTags(e.target.value)} /></FormControl>
            <Box sx={{ gridColumn: "1 / -1" }}>
              <Checkbox label="Put renewal and notice dates in my Calendar and Tasks" checked={f.remind !== false} onChange={(e) => set("remind", e.target.checked)} />
            </Box>
            <FormControl sx={{ gridColumn: "1 / -1" }}>
              <FormLabel>Notes</FormLabel>
              <Textarea minRows={2} value={f.notes ?? ""} onChange={(e) => set("notes", e.target.value)} />
            </FormControl>
          </Box>
          {err && <Typography color="danger" level="body-sm" role="alert" sx={{ mt: 1 }}>{err}</Typography>}
          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
            <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
            <Button loading={busy} disabled={!(f.title ?? "").trim()} onClick={() => void save()} data-testid="c-save">{initial ? "Save" : "Create contract"}</Button>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}
