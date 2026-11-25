// Field types, signer colours, default field sizes and the pre-send checks. The server repeats every check.
import type { CreateInput, FieldInput, FieldType, Request, SignerInput } from "./api";

export const FIELD_LABEL: Record<FieldType, string> = { signature: "Signature", initials: "Initials", date: "Date signed", text: "Text", checkbox: "Checkbox" };
export const SIGNER_COLORS = ["#2D7FF9", "#F82B60", "#20C933", "#FF9F40", "#8B46FF", "#18BFFF", "#FF08C2", "#666666"];
export const signerColor = (i: number) => SIGNER_COLORS[i % SIGNER_COLORS.length];

/** Default size of a field as fractions of an A4-like page. */
export function defaultSize(t: FieldType): { w: number; h: number } {
  switch (t) {
    case "signature": return { w: 0.28, h: 0.055 };
    case "initials": return { w: 0.1, h: 0.045 };
    case "date": return { w: 0.2, h: 0.03 };
    case "text": return { w: 0.28, h: 0.03 };
    case "checkbox": return { w: 0.03, h: 0.022 };
  }
}

/** A field centred at (cx, cy) but kept inside the page. */
export function placeField(t: FieldType, cx: number, cy: number): { x: number; y: number; w: number; h: number } {
  const { w, h } = defaultSize(t);
  return { x: Math.min(Math.max(0, cx - w / 2), 1 - w), y: Math.min(Math.max(0, cy - h / 2), 1 - h), w, h };
}

const EMAIL = /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/;

/** The problems the server would refuse, found before sending. Empty means ready. */
export function draftProblems(d: Partial<CreateInput>): string[] {
  const out: string[] = [];
  if (!d.title?.trim()) out.push("Give the request a title.");
  if (!d.file_id) out.push("Choose a PDF from Drive.");
  const signers = d.signers ?? [];
  if (signers.length === 0) out.push("Add at least one person.");
  const seen = new Set<string>();
  signers.forEach((s: SignerInput, i) => {
    if (!s.name.trim()) out.push(`Person ${i + 1} needs a name.`);
    if (!EMAIL.test(s.email.trim())) out.push(`${s.name || `Person ${i + 1}`} needs a valid email.`);
    const e = s.email.trim().toLowerCase();
    if (seen.has(e)) out.push(`${e} appears twice.`);
    seen.add(e);
  });
  if (signers.length && !signers.some((s) => (s.role ?? "signer") === "signer")) out.push("At least one person must sign, not only receive a copy.");
  signers.forEach((s, i) => {
    if ((s.role ?? "signer") !== "signer") return;
    if (!(d.fields ?? []).some((f: FieldInput) => f.signer_index === i && f.type === "signature")) out.push(`${s.name || `Person ${i + 1}`} needs a signature field on the document.`);
  });
  return out;
}

/** Drops fields of people who were removed and renumbers the rest. */
export function removeSigner(fields: FieldInput[], index: number): FieldInput[] {
  return fields.filter((f) => f.signer_index !== index).map((f) => (f.signer_index > index ? { ...f, signer_index: f.signer_index - 1 } : f));
}

export const STATUS_LABEL: Record<Request["status"], string> = { draft: "Draft", sent: "Out for signature", completed: "Completed", declined: "Declined", voided: "Voided", expired: "Expired" };
export const STATUS_COLOR: Record<Request["status"], "neutral" | "primary" | "success" | "danger" | "warning"> = { draft: "neutral", sent: "primary", completed: "success", declined: "danger", voided: "neutral", expired: "warning" };

/** "Waiting for Ravi" style summary of who is next. */
export function waitingOn(r: Request): string {
  if (r.status !== "sent") return "";
  const pending = (r.signers ?? []).filter((s) => s.role === "signer" && s.status !== "signed");
  if (!pending.length) return "";
  const next = r.sequential ? [...pending].sort((a, b) => a.order - b.order)[0] : null;
  return next ? `Waiting for ${next.name}` : `Waiting for ${pending.map((s) => s.name).join(", ")}`;
}

/** Picks a readable typed-signature name: the signer's name by default. */
export function typedDefault(name: string): string { return name.trim().slice(0, 80); }

/** True when a canvas drawing has enough ink to be a signature. */
export function hasInk(data: Uint8ClampedArray, minPixels = 40): boolean {
  let n = 0;
  for (let i = 3; i < data.length; i += 4) if (data[i] > 20 && ++n >= minPixels) return true;
  return false;
}
