import { PersonSelect } from "../../components/PeoplePicker";
import { useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Textarea, Typography } from "@mui/joy";
import { DrivePicker } from "../drive/DrivePicker";
import { PdfPages } from "../know/PdfPages";
import { signatures, type CreateInput, type FieldInput, type FieldType, type SignerInput } from "./api";
import { draftProblems, FIELD_LABEL, placeField, removeSigner, signerColor } from "./logic";

type Step = 0 | 1 | 2;

export function NewRequest({ onClose, onCreated, initialFile }: { onClose: () => void; onCreated: (id: string) => void; initialFile?: { id: string; name: string; contract_id?: string } }) {
  const [step, setStep] = useState<Step>(0);
  const [file, setFile] = useState(initialFile ?? null as { id: string; name: string } | null);
  const [pick, setPick] = useState(false);
  const [title, setTitle] = useState(initialFile ? initialFile.name.replace(/\.pdf$/i, "") : "");
  const [message, setMessage] = useState("");
  const [signers, setSigners] = useState<SignerInput[]>([{ name: "", email: "", role: "signer" }]);
  const [fields, setFields] = useState<FieldInput[]>([]);
  const [tool, setTool] = useState<FieldType>("signature");
  const [who, setWho] = useState(0);
  const [sequential, setSequential] = useState(false);
  const [expire, setExpire] = useState(14);
  const [people, setPeople] = useState<{ id: string; name: string; email: string }[]>([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => { signatures.people().then((r) => setPeople(r.people)).catch(() => undefined); }, []);
  const draft: CreateInput = { title, message, file_id: file?.id ?? "", signers, fields, sequential, expire_days: expire, contract_id: initialFile?.contract_id };
  const problems = draftProblems(draft);
  const setSigner = (i: number, p: Partial<SignerInput>) => setSigners(signers.map((s, j) => (j === i ? { ...s, ...p } : s)));
  const create = async (send: boolean) => {
    setBusy(true);
    setErr("");
    try {
      const r = await signatures.create(draft);
      if (send) await signatures.send(r.id);
      onCreated(r.id);
    } catch (e) { setErr((e as Error).message); setBusy(false); }
  };
  const signerOnly = signers.map((s, i) => ({ s, i })).filter((x) => (x.s.role ?? "signer") === "signer");
  const overlay = (page: number) => (
    <>
      {fields.map((f, i) => f.page === page && (
        <Box key={i} data-testid="placed-field" sx={{ position: "absolute", left: `${f.x * 100}%`, top: `${f.y * 100}%`, width: `${f.w * 100}%`, height: `${f.h * 100}%`, border: "2px solid", borderColor: signerColor(f.signer_index), bgcolor: signerColor(f.signer_index) + "22", borderRadius: "3px", fontSize: 11, color: signerColor(f.signer_index), display: "flex", alignItems: "center", justifyContent: "space-between", px: 0.3, overflow: "hidden" }} onClick={(e) => e.stopPropagation()}>
          <span>{FIELD_LABEL[f.type]}</span>
          <Box component="button" aria-label="Remove field" onClick={() => setFields(fields.filter((_, j) => j !== i))} sx={{ all: "unset", cursor: "pointer", px: 0.3 }}>✕</Box>
        </Box>
      ))}
    </>
  );
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 980, maxWidth: "100vw", height: "96vh", maxHeight: "96vh", overflow: "auto", "@media (max-width: 600px)": { width: "100vw", height: "100vh", maxHeight: "100vh", borderRadius: 0 } }} role="dialog" aria-label="New signature request">
        <ModalClose />
        <Typography level="title-lg">New signature request</Typography>
        <Box sx={{ display: "flex", gap: 1, my: 1 }}>
          {["Document", "People", "Place fields"].map((l, i) => <Chip key={l} variant={step === i ? "solid" : "soft"} color={step === i ? "primary" : "neutral"} onClick={() => i <= step || (i === 1 && file) || (i === 2 && file && signers.length) ? setStep(i as Step) : undefined}>{i + 1}. {l}</Chip>)}
        </Box>
        {step === 0 && (
          <Box sx={{ maxWidth: 520 }}>
            <FormControl sx={{ mb: 1.5 }}><FormLabel>PDF document</FormLabel>
              <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                <Button variant="outlined" onClick={() => setPick(true)} data-testid="pick-pdf">{file ? "Change file" : "Choose from Drive"}</Button>
                <Typography level="body-sm">{file?.name ?? "No file chosen"}</Typography>
              </Box>
              <Typography level="body-xs">Only PDF files you own can be sent. Upload it to Drive first.</Typography>
            </FormControl>
            <FormControl sx={{ mb: 1.5 }}><FormLabel>Title</FormLabel><Input value={title} onChange={(e) => setTitle(e.target.value)} data-testid="req-title" /></FormControl>
            <FormControl sx={{ mb: 1.5 }}><FormLabel>Message to the signers (optional)</FormLabel><Textarea minRows={3} value={message} onChange={(e) => setMessage(e.target.value)} /></FormControl>
            <Box sx={{ display: "flex", justifyContent: "flex-end" }}><Button disabled={!file || !title.trim()} onClick={() => setStep(1)} data-testid="next-people">Next: people</Button></Box>
          </Box>
        )}
        {step === 1 && (
          <Box sx={{ maxWidth: 700 }}>
            {signers.map((s, i) => (
              <Box key={i} sx={{ display: "flex", gap: 0.7, mb: 1, flexWrap: "wrap", alignItems: "center" }}>
                <Box sx={{ width: 14, height: 14, borderRadius: "50%", bgcolor: signerColor(i) }} />
                <Input size="sm" sx={{ flex: 1, minWidth: 130 }} placeholder="Name" value={s.name} onChange={(e) => setSigner(i, { name: e.target.value })} slotProps={{ input: { "aria-label": `Name of person ${i + 1}` } }} />
                <Input size="sm" sx={{ flex: 1, minWidth: 170 }} placeholder="Email" value={s.email} onChange={(e) => setSigner(i, { email: e.target.value })} slotProps={{ input: { "aria-label": `Email of person ${i + 1}` } }} />
                <Select size="sm" value={s.role ?? "signer"} onChange={(_, v) => v && setSigner(i, { role: v as "signer" | "cc" })} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}><Option value="signer">Signs</Option><Option value="cc">Gets a copy</Option></Select>
                <PersonSelect value="" known={people} onChange={(id, item) => { if (id && item) setSigner(i, { name: item.label, email: item.email ?? "", user_id: id }); }} placeholder="Pick a colleague" ariaLabel="Person" />
                {signers.length > 1 && <IconButton size="sm" variant="plain" aria-label={`Remove person ${i + 1}`} onClick={() => { setSigners(signers.filter((_, j) => j !== i)); setFields(removeSigner(fields, i)); }}>✕</IconButton>}
              </Box>
            ))}
            <Button size="sm" variant="soft" onClick={() => setSigners([...signers, { name: "", email: "", role: "signer" }])} data-testid="add-person">＋ Add person</Button>
            <Box sx={{ mt: 2 }}>
              <Checkbox label="Sign in order (each person is asked after the one before)" checked={sequential} onChange={(e) => setSequential(e.target.checked)} />
              <FormControl sx={{ mt: 1, maxWidth: 240 }}><FormLabel>Expires after (days, 0 = never)</FormLabel><Input type="number" value={expire} onChange={(e) => setExpire(Math.max(0, Math.min(365, Number(e.target.value))))} /></FormControl>
            </Box>
            <Box sx={{ display: "flex", justifyContent: "space-between", mt: 2 }}><Button variant="plain" onClick={() => setStep(0)}>Back</Button><Button disabled={signers.some((s) => !s.name.trim() || !s.email.trim())} onClick={() => { setWho(signerOnly[0]?.i ?? 0); setStep(2); }} data-testid="next-fields">Next: place fields</Button></Box>
          </Box>
        )}
        {step === 2 && file && (
          <Box sx={{ display: "flex", gap: 2, flexDirection: { xs: "column", md: "row" } }}>
            <Box sx={{ width: { md: 230 }, flexShrink: 0 }}>
              <FormLabel>Who is it for</FormLabel>
              <Select size="sm" value={who} onChange={(_, v) => v !== null && setWho(v)} sx={{ mb: 1 }} slotProps={{ listbox: { sx: { zIndex: 2000 } } }}>
                {signerOnly.map(({ s, i }) => <Option key={i} value={i}>{s.name || `Person ${i + 1}`}</Option>)}
              </Select>
              <FormLabel>Field to place</FormLabel>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mb: 1 }}>
                {(Object.keys(FIELD_LABEL) as FieldType[]).map((t) => <Chip key={t} variant={tool === t ? "solid" : "outlined"} color="primary" onClick={() => setTool(t)} data-testid={`tool-${t}`}>{FIELD_LABEL[t]}</Chip>)}
              </Box>
              <Typography level="body-xs">Click on the page to put a {FIELD_LABEL[tool].toLowerCase()} field for {signers[who]?.name || "this person"}.</Typography>
              <Typography level="body-xs" sx={{ mt: 1 }}>{fields.length} field{fields.length === 1 ? "" : "s"} placed.</Typography>
              {problems.length > 0 && <Box sx={{ mt: 1.5, p: 1, bgcolor: "warning.softBg", borderRadius: "sm", fontSize: 12 }} role="status">{problems.map((p) => <div key={p}>• {p}</div>)}</Box>}
              {err && <Typography color="danger" level="body-sm" role="alert" sx={{ mt: 1 }}>{err}</Typography>}
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1, mt: 2 }}>
                <Button variant="soft" disabled={problems.length > 0} loading={busy} onClick={() => void create(false)}>Save as draft</Button>
                <Button disabled={problems.length > 0} loading={busy} onClick={() => void create(true)} data-testid="send-request">Send for signature</Button>
                <Button variant="plain" onClick={() => setStep(1)}>Back</Button>
              </Box>
            </Box>
            <Box sx={{ flex: 1, minWidth: 0, bgcolor: "background.level1", p: 1, borderRadius: "md", maxHeight: "80vh", overflow: "auto" }}>
              <PdfPages url={`/api/v1/drive/files/${file.id}/content`} maxWidth={700} overlay={overlay}
                onPageClick={(page, x, y) => setFields([...fields, { signer_index: who, type: tool, page, ...placeField(tool, x, y), required: true }])} />
            </Box>
          </Box>
        )}
        {pick && <DrivePicker mode="file" title="Choose the PDF" confirmLabel="Use this file" onClose={() => setPick(false)} onPick={(f) => { setPick(false); const d = f as { id: string; name: string; mime_type?: string }; if (d.mime_type && d.mime_type !== "application/pdf") { window.alert("Only PDF files can be sent for signature."); return; } setFile({ id: d.id, name: d.name }); setTitle((t) => t || d.name.replace(/\.pdf$/i, "")); }} />}
      </ModalDialog>
    </Modal>
  );
}
