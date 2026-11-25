import { useEffect, useMemo, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { Alert, Box, Button, Checkbox, Chip, CircularProgress, FormControl, FormLabel, Input, Sheet, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { ACCENT, JOB_TYPES, salaryRange } from "./logic";

/** Public career page at /careers/:org. No sign-in. */
export function CareersPage() {
  const { org = "" } = useParams();
  const [data, setData] = useState<Awaited<ReturnType<typeof api.publicJobs>> | null>(null);
  const [missing, setMissing] = useState(false);
  const [open, setOpen] = useState<api.PublicJob | null>(null);
  const [done, setDone] = useState(false);

  useEffect(() => {
    api.publicJobs(org).then(setData).catch(() => setMissing(true));
  }, [org]);

  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "#f3f6f5", py: 4, px: 1.5 }}>
      <Box sx={{ maxWidth: 820, mx: "auto" }}>
        {!data && !missing && <CircularProgress />}
        {missing && (
          <Typography level="h3" data-testid="careers-missing">
            Careers are not available here
          </Typography>
        )}
        {data && (
          <>
            <Typography level="h2" sx={{ color: ACCENT }} data-testid="careers-title">
              {data.title}
            </Typography>
            {data.blurb && <Typography sx={{ mb: 2 }}>{data.blurb}</Typography>}
            {done && (
              <Alert color="success" sx={{ mb: 2 }} data-testid="careers-done">
                Thank you. Your application was sent.
              </Alert>
            )}
            {!open && data.jobs.length === 0 && <Typography>There are no open positions right now.</Typography>}
            {!open &&
              data.jobs.map((j) => (
                <Sheet key={j.id} variant="outlined" sx={{ p: 2, mb: 1.5, borderRadius: "md", bgcolor: "#fff" }} data-testid="careers-job">
                  <Typography level="title-lg">{j.title}</Typography>
                  <Typography level="body-sm">
                    {[j.department, j.location, JOB_TYPES[j.employment_type] ?? j.employment_type].filter(Boolean).join(" · ")}
                  </Typography>
                  {salaryRange(j) && <Typography level="body-sm">{salaryRange(j)}</Typography>}
                  <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap", my: 1 }}>
                    {j.skills.map((s) => (
                      <Chip key={s} size="sm" variant="soft">
                        {s}
                      </Chip>
                    ))}
                  </Box>
                  <Button size="sm" sx={{ bgcolor: ACCENT }} onClick={() => { setOpen(j); setDone(false); }} data-testid="careers-apply-open">
                    View and apply
                  </Button>
                </Sheet>
              ))}
            {open && (
              <ApplyForm org={org} job={open} privacy={data.privacy_text} onBack={() => setOpen(null)} onDone={() => { setOpen(null); setDone(true); }} />
            )}
          </>
        )}
      </Box>
    </Box>
  );
}

function ApplyForm({ org, job, privacy, onBack, onDone }: { org: string; job: api.PublicJob; privacy: string; onBack: () => void; onDone: () => void }) {
  const started = useRef(Date.now());
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [consent, setConsent] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [f, setF] = useState({ name: "", email: "", phone: "", cover_note: "" });
  const tooBig = useMemo(() => !!file && file.size > 5 * 1024 * 1024, [file]);
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value });

  const submit = async (ev: React.FormEvent<HTMLFormElement>) => {
    ev.preventDefault();
    setErr("");
    if (!consent) return setErr("Please accept the privacy statement to apply.");
    if (tooBig) return setErr("The resume is larger than 5 MB.");
    const fd = new FormData();
    Object.entries(f).forEach(([k, v]) => fd.append(k, v));
    fd.append("consent", "true");
    fd.append("started_at", String(started.current));
    // Hidden honeypot: real people never see or fill this field.
    fd.append("website", (ev.currentTarget.elements.namedItem("website") as HTMLInputElement)?.value ?? "");
    if (file) fd.append("resume", file);
    setBusy(true);
    try {
      await api.publicApply(org, job.id, fd);
      onDone();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet variant="outlined" sx={{ p: 2.5, borderRadius: "md", bgcolor: "#fff" }}>
      <Button size="sm" variant="plain" onClick={onBack}>
        Back to all jobs
      </Button>
      <Typography level="h3">{job.title}</Typography>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        {[job.department, job.location].filter(Boolean).join(" · ")}
      </Typography>
      <Typography sx={{ whiteSpace: "pre-wrap", mb: 2 }}>{job.description}</Typography>
      <form onSubmit={submit} data-testid="careers-form">
        <Box sx={{ display: "grid", gap: 1.5, maxWidth: 520 }}>
          <FormControl required>
            <FormLabel>Full name</FormLabel>
            <Input value={f.name} onChange={set("name")} slotProps={{ input: { "data-testid": "apply-name", maxLength: 120 } }} />
          </FormControl>
          <FormControl required>
            <FormLabel>Email</FormLabel>
            <Input type="email" value={f.email} onChange={set("email")} slotProps={{ input: { "data-testid": "apply-email", maxLength: 200 } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Phone</FormLabel>
            <Input value={f.phone} onChange={set("phone")} slotProps={{ input: { "data-testid": "apply-phone", maxLength: 32 } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Cover note</FormLabel>
            <Textarea minRows={3} value={f.cover_note} onChange={set("cover_note")} slotProps={{ textarea: { maxLength: 5000 } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Resume (PDF, Word, ODT, RTF or text, up to 5 MB)</FormLabel>
            <input type="file" accept=".pdf,.doc,.docx,.odt,.rtf,.txt" onChange={(e) => setFile(e.target.files?.[0] ?? null)} data-testid="apply-resume" />
          </FormControl>
          <div style={{ position: "absolute", left: "-10000px" }} aria-hidden="true">
            <label>
              Website
              <input name="website" tabIndex={-1} autoComplete="off" defaultValue="" />
            </label>
          </div>
          <Checkbox checked={consent} onChange={(e) => setConsent(e.target.checked)} label={privacy} data-testid="apply-consent" />
          {err && (
            <Alert color="danger" data-testid="apply-error">
              {err}
            </Alert>
          )}
          <Button type="submit" loading={busy} sx={{ bgcolor: ACCENT }} data-testid="apply-submit">
            Submit application
          </Button>
        </Box>
      </form>
    </Sheet>
  );
}

/** Offer e-accept page at /careers/offer/:token. No sign-in. */
export function OfferPage() {
  const { token = "" } = useParams();
  const [o, setO] = useState<Awaited<ReturnType<typeof api.publicOffer>> | null>(null);
  const [missing, setMissing] = useState(false);
  const [name, setName] = useState("");
  const [err, setErr] = useState("");
  const [result, setResult] = useState("");

  const load = () => api.publicOffer(token).then(setO).catch(() => setMissing(true));
  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  const respond = async (accept: boolean) => {
    setErr("");
    try {
      await api.publicOfferRespond(token, accept, name);
      setResult(accept ? "You accepted the offer. We will be in touch about your start." : "You declined the offer.");
      load();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "#f3f6f5", py: 4, px: 1.5 }}>
      <Box sx={{ maxWidth: 760, mx: "auto" }}>
        {!o && !missing && <CircularProgress />}
        {missing && <Typography level="h3" data-testid="offer-missing">This offer link is not available</Typography>}
        {o && (
          <Sheet variant="outlined" sx={{ p: 3, borderRadius: "md", bgcolor: "#fff" }}>
            <Typography level="h3" sx={{ color: ACCENT }}>
              Offer: {o.job_title}
            </Typography>
            <Typography level="body-sm" sx={{ mb: 2 }}>
              {o.company}
            </Typography>
            <Typography sx={{ whiteSpace: "pre-wrap", mb: 2 }} data-testid="offer-body">
              {o.body}
            </Typography>
            <Button component="a" href={api.publicOfferPdfUrl(token)} target="_blank" size="sm" variant="outlined" sx={{ mb: 2 }}>
              Download the letter (PDF)
            </Button>
            {result && <Alert color="success" data-testid="offer-result">{result}</Alert>}
            {err && <Alert color="danger">{err}</Alert>}
            {o.status === "sent" && !o.expired && !result && (
              <Box sx={{ display: "grid", gap: 1.5, maxWidth: 420 }}>
                <FormControl>
                  <FormLabel>Type your full name to accept</FormLabel>
                  <Input value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "offer-name" } }} />
                </FormControl>
                <Box sx={{ display: "flex", gap: 1 }}>
                  <Button onClick={() => respond(true)} sx={{ bgcolor: ACCENT }} data-testid="offer-accept">
                    Accept offer
                  </Button>
                  <Button variant="outlined" color="neutral" onClick={() => respond(false)} data-testid="offer-decline">
                    Decline
                  </Button>
                </Box>
              </Box>
            )}
            {o.status !== "sent" && !result && <Alert color="neutral">This offer is {o.status}.</Alert>}
            {o.expired && <Alert color="warning">This offer has expired. Please contact the hiring team.</Alert>}
          </Sheet>
        )}
      </Box>
    </Box>
  );
}
