export interface FormExtras {
  theme_color: string;
  theme_bg: string;
  notify_owner: boolean;
}

export const emptyExtras = (): FormExtras => ({ theme_color: "", theme_bg: "", notify_owner: false });

/** Preset themes: accent colour and a soft page background that keeps text readable. */
export const THEMES: { name: string; color: string; bg: string }[] = [
  { name: "Adrient green", color: "#126C5A", bg: "#eef4f2" },
  { name: "Blue", color: "#1a73e8", bg: "#e8f0fe" },
  { name: "Purple", color: "#8e24aa", bg: "#f3e5f5" },
  { name: "Orange", color: "#e8710a", bg: "#fef3e8" },
  { name: "Red", color: "#d93025", bg: "#fce8e6" },
  { name: "Slate", color: "#455a64", bg: "#eceff1" },
];

export const isHexColor = (c: string) => /^#[0-9a-fA-F]{6}$/.test(c);

/** Darkens a #RRGGBB colour by a factor (0..1) for hover states. */
export function darken(hex: string, amount = 0.12): string {
  if (!isHexColor(hex)) return hex;
  const n = parseInt(hex.slice(1), 16);
  const ch = (v: number) => Math.max(0, Math.min(255, Math.round(v * (1 - amount))));
  const r = ch((n >> 16) & 255);
  const g = ch((n >> 8) & 255);
  const b = ch(n & 255);
  return "#" + [r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("");
}

/** CSS variable overrides for a theme; only valid colours are ever used. */
export function themeVars(e: Pick<FormExtras, "theme_color" | "theme_bg">): Record<string, string> {
  const v: Record<string, string> = {};
  if (isHexColor(e.theme_color)) {
    v["--joy-palette-primary-solidBg"] = e.theme_color;
    v["--joy-palette-primary-solidHoverBg"] = darken(e.theme_color);
    v["--joy-palette-primary-solidActiveBg"] = darken(e.theme_color, 0.2);
    v["--joy-palette-primary-500"] = e.theme_color;
    v["--joy-palette-primary-plainColor"] = e.theme_color;
    v["--joy-palette-primary-outlinedColor"] = e.theme_color;
    v["--joy-palette-primary-outlinedBorder"] = e.theme_color;
  }
  if (isHexColor(e.theme_bg)) v["--joy-palette-background-level1"] = e.theme_bg;
  return v;
}

/** The iframe snippet for embedding a form. */
export function embedSnippet(origin: string, formId: string, height = 900): string {
  const src = `${origin.replace(/\/$/, "")}/forms/d/${encodeURIComponent(formId)}/viewform`;
  return `<iframe src="${src}" width="100%" height="${Math.max(300, Math.floor(height))}" frameborder="0" marginheight="0" marginwidth="0" title="Form">Loading...</iframe>`;
}

/** Splits pasted addresses on commas, semicolons and whitespace. */
export function splitEmails(text: string): string[] {
  const seen = new Set<string>();
  for (const p of text.split(/[\s,;]+/)) {
    const e = p.trim().toLowerCase();
    if (e) seen.add(e);
  }
  return [...seen];
}

export const isEmail = (s: string) => /^[^\s@<>"]+@[^\s@<>"]+\.[^\s@<>"]+$/.test(s);
