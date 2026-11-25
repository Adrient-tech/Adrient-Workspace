import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { Box, Button, Checkbox, Chip, Container, Input, Modal, ModalDialog, Textarea, Typography, Tab, TabList, Tabs, TabPanel } from "@mui/joy";
import { PdfPages } from "../know/PdfPages";
import { signatures, type Field, type PublicDoc } from "./api";
import { FIELD_LABEL, hasInk, typedDefault } from "./logic";

function SignaturePad({ onChange }: { onChange: (dataUrl: string) => void }) {
  const ref = useRef<HTMLCanvasElement>(null);
  const drawing = useRef(false);
  const pos = (e: React.PointerEvent) => { const r = ref.current!.getBoundingClientRect(); return { x: ((e.clientX - r.left) / r.width) * ref.current!.width, y: ((e.clientY - r.top) / r.height) * ref.current!.height }; };
  const down = (e: React.PointerEvent) => { drawing.current = true; ref.current!.setPointerCapture(e.pointerId); const c = ref.current!.getContext("2d")!; const p = pos(e); c.beginPath(); c.moveTo(p.x, p.y); };
  const move = (e: React.PointerEvent) => { if (!drawing.current) return; const c = ref.current!.getContext("2d")!; c.lineWidth = 3; c.lineCap = "round"; c.strokeStyle = "#0b1f5c"; const p = pos(e); c.lineTo(p.x, p.y); c.stroke(); };
  const up = () => {
    if (!drawing.current) return;
    drawing.current = false;
    const c = ref.current!;
    const img = c.getContext("2d")!.getImageData(0, 0, c.width, c.height);
    onChange(hasInk(img.data) ? c.toDataURL("image/png") : "");
  };
  return (
    <Box>
      <Box sx={{ border: "1px dashed", borderColor: "neutral.400", borderRadius: "sm", bgcolor: "#fff", touchAction: "none" }}>
        <canvas ref={ref} width={560} height={180} style={{ width: "100%", height: 150, display: "block", touchAction: "none" }} onPointerDown={down} onPointerMove={move} onPointerUp={up} onPointerLeave={up} aria-label="Draw your signature here" data-testid="sig-pad" />
      </Box>
      <Button size="sm" variant="plain" onClick={() => { const c = ref.current!; c.getContext("2d")!.clearRect(0, 0, c.width, c.height); onChange(""); }}>Clear</Button>
    </Box>
  );
}

export default function SignPage() {
  const { token = "" } = useParams();
  const [doc, setDoc] = useState<PublicDoc | null>(null);
  const [err, setErr] = useState("");
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<"typed" | "drawn">("typed");
  const [typed, setTyped] = useState("");
  const [drawn, setDrawn] = useState("");
  const [consent, setConsent] = useState(false);
  const [values, setValues] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [declining, setDeclining] = useState(false);
  const [reason, setReason] = useState("");
  const [done, setDone] = useState("");
  useEffect(() => {
    signatures.publicDoc(token).then((d) => { setDoc(d); setTyped(typedDefault(d.signer.name)); }).catch((e: Error) => setErr(e.message === "not found" ? "This link is not valid any more. It may have been used, replaced by a newer link, cancelled or expired." : e.message));
  }, [token]);
  if (err) return <Container maxWidth="sm" sx={{ py: 8 }}><Typography level="h3">We cannot open this document</Typography><Typography sx={{ mt: 1 }}>{err}</Typography></Container>;
  if (!doc) return <Container maxWidth="sm" sx={{ py: 8 }}><Typography>Loading…</Typography></Container>;
  const mine = doc.fields;
  const textFields = mine.filter((f) => f.type === "text" || f.type === "checkbox");
  const missing = () => {
    for (const f of textFields) {
      if (f.required && f.type === "text" && !(values[f.id] ?? "").trim()) return `Fill in “${f.label || "the text field"}”.`;
      if (f.required && f.type === "checkbox" && values[f.id] !== "true") return `Tick “${f.label || "the box"}”.`;
    }
    if (kind === "typed" && !typed.trim()) return "Type your name.";
    if (kind === "drawn" && !drawn) return "Draw your signature.";
    if (!consent) return "Agree to sign electronically.";
    return "";
  };
  const submit = async () => {
    const m = missing();
    if (m) { setErr(""); window.alert(m); return; }
    setBusy(true);
    try {
      const d = await signatures.publicSign(token, { consent, signature_kind: kind, signature_data: kind === "typed" ? typed : drawn, values });
      setDoc(d); setOpen(false); setDone("Thank you. Your signature was recorded.");
    } catch (e) { window.alert((e as Error).message); } finally { setBusy(false); }
  };
  const overlay = (page: number) => (
    <>
      {mine.filter((f) => f.page === page).map((f: Field) => (
        <Box key={f.id} data-testid="sign-field" onClick={(e) => { e.stopPropagation(); if (doc.can_sign && (f.type === "signature" || f.type === "initials")) setOpen(true); }}
          sx={{ position: "absolute", left: `${f.x * 100}%`, top: `${f.y * 100}%`, width: `${f.w * 100}%`, height: `${f.h * 100}%`, border: "2px solid #2D7FF9", bgcolor: "rgba(45,127,249,.12)", borderRadius: "3px", display: "flex", alignItems: "center", justifyContent: "center", fontSize: 12, color: "#1b4fb5", cursor: doc.can_sign ? "pointer" : "default", overflow: "hidden" }}>
          {f.type === "text" && doc.can_sign ? (
            <input aria-label={f.label || "Text"} value={values[f.id] ?? ""} maxLength={500} onChange={(e) => setValues({ ...values, [f.id]: e.target.value })} onClick={(e) => e.stopPropagation()} placeholder={f.label || "Type here"} style={{ width: "100%", height: "100%", border: 0, background: "transparent", font: "inherit", padding: "0 4px" }} />
          ) : f.type === "checkbox" && doc.can_sign ? (
            <input type="checkbox" aria-label={f.label || "Checkbox"} checked={values[f.id] === "true"} onChange={(e) => setValues({ ...values, [f.id]: String(e.target.checked) })} onClick={(e) => e.stopPropagation()} />
          ) : f.type === "date" ? "Date" : FIELD_LABEL[f.type]}
        </Box>
      ))}
    </>
  );
  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "background.level1" }}>
      <Box sx={{ position: "sticky", top: 0, zIndex: 20, bgcolor: "background.surface", borderBottom: "1px solid", borderColor: "divider", px: 2, py: 1, display: "flex", gap: 1.5, alignItems: "center", flexWrap: "wrap" }}>
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography level="title-md" noWrap>{doc.title}</Typography>
          <Typography level="body-xs">From {doc.sender} · for {doc.signer.name}</Typography>
        </Box>
        {doc.can_sign && <Button onClick={() => setOpen(true)} data-testid="start-signing">Sign</Button>}
        {doc.can_sign && <Button variant="outlined" color="neutral" onClick={() => setDeclining(true)}>Decline</Button>}
        {doc.done_signed && <Chip color="success" variant="soft">You signed</Chip>}
        {doc.waiting && <Chip variant="soft">Waiting for earlier signers</Chip>}
        {doc.status !== "sent" && <Chip variant="soft" color={doc.status === "completed" ? "success" : "warning"}>{doc.status}</Chip>}
      </Box>
      {done && <Box sx={{ p: 1.5, bgcolor: "success.softBg", textAlign: "center" }} role="status">{done}</Box>}
      {doc.message && <Container maxWidth="md" sx={{ pt: 2 }}><Box sx={{ p: 1.5, bgcolor: "background.surface", borderRadius: "md", whiteSpace: "pre-wrap" }}>{doc.message}</Box></Container>}
      <Container maxWidth="md" sx={{ py: 2, px: { xs: 0.5, sm: 2 } }}>
        <PdfPages url={signatures.publicPdf(token)} overlay={overlay} />
        {doc.others.length > 0 && <Typography level="body-xs" sx={{ mt: 2 }}>Others: {doc.others.join(", ")}</Typography>}
        <Typography level="body-xs" sx={{ mt: 1, color: "text.tertiary" }}>Signing is recorded with your IP address and time. You can download the signed copy from the sender after everyone has signed.</Typography>
      </Container>
      {open && (
        <Modal open onClose={() => setOpen(false)}>
          <ModalDialog sx={{ width: 520, maxWidth: "96vw", maxHeight: "96vh", overflow: "auto" }} role="dialog" aria-label="Sign">
            <Typography level="title-lg">Sign “{doc.title}”</Typography>
            <Tabs value={kind} onChange={(_, v) => v && setKind(v as "typed" | "drawn")}>
              <TabList><Tab value="typed">Type</Tab><Tab value="drawn">Draw</Tab></TabList>
              <TabPanel value="typed"><Input value={typed} onChange={(e) => setTyped(e.target.value)} slotProps={{ input: { "aria-label": "Type your name", maxLength: 80 } }} /><Typography sx={{ mt: 1, fontFamily: "'Times New Roman', serif", fontStyle: "italic", fontSize: 30, color: "#0b1f5c", minHeight: 44 }}>{typed}</Typography></TabPanel>
              <TabPanel value="drawn"><SignaturePad onChange={setDrawn} /></TabPanel>
            </Tabs>
            <Checkbox sx={{ mt: 1 }} checked={consent} onChange={(e) => setConsent(e.target.checked)} label="I agree to sign this document electronically and that this counts as my signature." data-testid="consent" />
            <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1.5 }}>
              <Button variant="plain" color="neutral" onClick={() => setOpen(false)}>Cancel</Button>
              <Button loading={busy} onClick={() => void submit()} data-testid="finish-signing">Finish signing</Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
      {declining && (
        <Modal open onClose={() => setDeclining(false)}>
          <ModalDialog sx={{ width: 460, maxWidth: "96vw" }} role="dialog" aria-label="Decline to sign">
            <Typography level="title-lg">Decline to sign</Typography>
            <Typography level="body-sm">The sender is told and the request stops for everyone.</Typography>
            <Textarea minRows={3} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Reason (optional)" sx={{ mt: 1 }} />
            <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1.5 }}>
              <Button variant="plain" color="neutral" onClick={() => setDeclining(false)}>Cancel</Button>
              <Button color="danger" onClick={() => void signatures.publicDecline(token, reason).then(() => signatures.publicDoc(token).catch(() => null)).then((d) => { setDeclining(false); if (d) setDoc(d); else setErr("You declined this request."); }).catch((e: Error) => window.alert(e.message))}>Decline</Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}
