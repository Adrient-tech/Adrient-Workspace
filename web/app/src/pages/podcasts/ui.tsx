/**
 * Small light-theme building blocks of the Stories app. Colours are explicit
 * (the app is light only), so nothing here depends on the Joy colour scheme.
 */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { Box } from "@mui/joy";
import CloseIcon from "@mui/icons-material/Close";

export const C = {
  bg: "#F6F4EF",
  surface: "#FFFFFF",
  ink: "#16211E",
  muted: "#66726E",
  faint: "#9AA5A1",
  line: "#E7E3D9",
  accent: "#126C5A",
  accentDark: "#0C4A3E",
  accentSoft: "#E2F0EC",
  saffron: "#F2A21B",
  danger: "#B3261E",
  dangerSoft: "#FBEAE8",
  chip: "#EFEBE1",
} as const;

export const FONT_DISPLAY = '"Plus Jakarta Sans", "Inter", system-ui, sans-serif';

export function useIsMobile(): boolean {
  const [m, setM] = useState(() => typeof window !== "undefined" && window.matchMedia("(max-width: 800px)").matches);
  useEffect(() => {
    const q = window.matchMedia("(max-width: 800px)");
    const f = () => setM(q.matches);
    q.addEventListener("change", f);
    return () => q.removeEventListener("change", f);
  }, []);
  return m;
}

/** hue,hue,angle -> CSS gradient. */
export function gradientFor(style: string): string {
  const [a, b, ang] = (style || "172,205,135").split(",").map((x) => parseInt(x, 10));
  const h1 = Number.isFinite(a) ? a : 172;
  const h2 = Number.isFinite(b) ? b : 205;
  const deg = Number.isFinite(ang) ? ang : 135;
  return `linear-gradient(${deg}deg, hsl(${h1} 52% 34%), hsl(${h2} 58% 52%))`;
}

export function randomCoverStyle(): string {
  const h = Math.floor(Math.random() * 360);
  return `${h},${(h + 40 + Math.floor(Math.random() * 80)) % 360},${120 + Math.floor(Math.random() * 90)}`;
}

interface CoverProps {
  title: string;
  url?: string;
  style: string;
  size?: number | string;
  radius?: number;
  sx?: Record<string, unknown>;
}

/** Cover art: an uploaded image, or a gradient with the title set in big type. */
export function Cover({ title, url, style, size = 120, radius = 12, sx }: CoverProps) {
  const px = typeof size === "number" ? size : undefined;
  const font = px ? Math.max(11, Math.round(px / 8.2)) : 18;
  return (
    <Box
      aria-label={`Cover of ${title}`}
      role="img"
      sx={{
        width: size, height: size, aspectRatio: "1 / 1", borderRadius: `${radius}px`, flexShrink: 0, position: "relative", overflow: "hidden",
        background: url ? C.chip : gradientFor(style), boxShadow: "0 1px 2px rgba(22,33,30,.12), 0 6px 18px rgba(22,33,30,.10)", ...sx,
      }}
    >
      {url ? (
        <img src={url} alt="" style={{ width: "100%", height: "100%", objectFit: "cover", display: "block" }} />
      ) : (
        <Box sx={{ position: "absolute", inset: 0, display: "flex", alignItems: "flex-end", p: px && px < 70 ? 0.6 : 1.5 }}>
          <Box sx={{ position: "absolute", top: "-22%", right: "-18%", width: "70%", aspectRatio: "1", borderRadius: "50%", background: "rgba(255,255,255,.14)" }} />
          <Box
            sx={{
              position: "relative", color: "#fff", fontFamily: FONT_DISPLAY, fontWeight: 800, lineHeight: 1.08, letterSpacing: "-0.02em",
              fontSize: font, textShadow: "0 1px 8px rgba(0,0,0,.25)", display: "-webkit-box", WebkitLineClamp: px && px < 70 ? 2 : 5,
              WebkitBoxOrient: "vertical", overflow: "hidden", wordBreak: "break-word",
            }}
          >
            {title}
          </Box>
        </Box>
      )}
    </Box>
  );
}

type BtnProps = {
  children?: ReactNode;
  onClick?: (e: React.MouseEvent) => void;
  variant?: "primary" | "soft" | "ghost" | "danger" | "outline";
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
  title?: string;
  type?: "button" | "submit";
  startIcon?: ReactNode;
  testId?: string;
  "aria-pressed"?: boolean;
  "aria-label"?: string;
  sx?: Record<string, unknown>;
};

export function Btn({ children, onClick, variant = "soft", size = "md", disabled, title, type = "button", startIcon, testId, sx, ...rest }: BtnProps) {
  const pad = size === "lg" ? "12px 26px" : size === "sm" ? "5px 12px" : "8px 16px";
  const fs = size === "lg" ? 16 : size === "sm" ? 13 : 14;
  const look =
    variant === "primary" ? { bg: C.accent, fg: "#fff", bd: C.accent, hov: C.accentDark }
    : variant === "danger" ? { bg: C.dangerSoft, fg: C.danger, bd: "transparent", hov: "#F6D9D5" }
    : variant === "outline" ? { bg: "transparent", fg: C.ink, bd: C.line, hov: C.chip }
    : variant === "ghost" ? { bg: "transparent", fg: C.ink, bd: "transparent", hov: C.chip }
    : { bg: C.accentSoft, fg: C.accentDark, bd: "transparent", hov: "#D3E8E2" };
  return (
    <Box
      component="button"
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      data-testid={testId}
      aria-pressed={rest["aria-pressed"]}
      aria-label={rest["aria-label"]}
      sx={{
        display: "inline-flex", alignItems: "center", justifyContent: "center", gap: 0.75, padding: pad, fontSize: fs, fontWeight: 650, fontFamily: "inherit",
        borderRadius: "999px", border: `1px solid ${look.bd}`, background: look.bg, color: look.fg, cursor: disabled ? "default" : "pointer",
        opacity: disabled ? 0.5 : 1, whiteSpace: "nowrap", transition: "background .12s, transform .08s",
        "&:hover:not(:disabled)": { background: look.hov }, "&:active:not(:disabled)": { transform: "scale(.97)" },
        "&:focus-visible": { outline: `2px solid ${C.accent}`, outlineOffset: 2 }, ...sx,
      }}
    >
      {startIcon}
      {children}
    </Box>
  );
}

export function IconBtn({ children, onClick, label, size = 40, active, disabled, sx, testId }: {
  children: ReactNode; onClick?: (e: React.MouseEvent) => void; label: string; size?: number; active?: boolean; disabled?: boolean; sx?: Record<string, unknown>; testId?: string;
}) {
  return (
    <Box
      component="button"
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      disabled={disabled}
      data-testid={testId}
      sx={{
        width: size, height: size, borderRadius: "50%", border: "none", background: active ? C.accentSoft : "transparent", color: active ? C.accent : C.ink,
        display: "inline-flex", alignItems: "center", justifyContent: "center", cursor: disabled ? "default" : "pointer", opacity: disabled ? 0.4 : 1, flexShrink: 0,
        "&:hover:not(:disabled)": { background: C.chip }, "&:focus-visible": { outline: `2px solid ${C.accent}`, outlineOffset: 1 }, ...sx,
      }}
    >
      {children}
    </Box>
  );
}

export function Chip({ children, active, onClick, sx, testId }: { children: ReactNode; active?: boolean; onClick?: () => void; sx?: Record<string, unknown>; testId?: string }) {
  return (
    <Box
      component={onClick ? "button" : "span"}
      type={onClick ? "button" : undefined}
      onClick={onClick}
      data-testid={testId}
      aria-pressed={onClick ? !!active : undefined}
      sx={{
        display: "inline-flex", alignItems: "center", gap: 0.5, padding: "6px 14px", borderRadius: "999px", fontSize: 13.5, fontWeight: 600, fontFamily: "inherit",
        border: `1px solid ${active ? C.accent : C.line}`, background: active ? C.accent : C.surface, color: active ? "#fff" : C.ink,
        cursor: onClick ? "pointer" : "default", whiteSpace: "nowrap", "&:hover": onClick ? { borderColor: C.accent } : {}, ...sx,
      }}
    >
      {children}
    </Box>
  );
}

export function Skeleton({ w = "100%", h = 16, r = 8, sx }: { w?: number | string; h?: number | string; r?: number; sx?: Record<string, unknown> }) {
  return (
    <Box
      aria-hidden
      sx={{
        width: w, height: h, borderRadius: `${r}px`, background: `linear-gradient(90deg, ${C.chip} 0%, #F7F4EC 50%, ${C.chip} 100%)`, backgroundSize: "200% 100%",
        animation: "stories-shimmer 1.3s linear infinite", "@keyframes stories-shimmer": { "0%": { backgroundPosition: "200% 0" }, "100%": { backgroundPosition: "-200% 0" } }, flexShrink: 0, ...sx,
      }}
    />
  );
}

export function ShelfSkeleton() {
  return (
    <Box sx={{ display: "flex", gap: 2, overflow: "hidden" }} data-testid="skeleton">
      {[0, 1, 2, 3, 4].map((i) => (
        <Box key={i} sx={{ width: 170, flexShrink: 0 }}>
          <Skeleton w={170} h={170} r={14} />
          <Skeleton w="80%" h={14} sx={{ mt: 1.2 }} />
          <Skeleton w="50%" h={12} sx={{ mt: 0.8 }} />
        </Box>
      ))}
    </Box>
  );
}

export function Empty({ icon, title, hint, action }: { icon?: ReactNode; title: string; hint?: string; action?: ReactNode }) {
  return (
    <Box sx={{ textAlign: "center", py: 6, px: 2, color: C.muted }} data-testid="empty-state">
      {icon && <Box sx={{ fontSize: 44, color: C.faint, mb: 1, display: "flex", justifyContent: "center" }}>{icon}</Box>}
      <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 700, fontSize: 18, color: C.ink, mb: 0.5 }}>{title}</Box>
      {hint && <Box sx={{ fontSize: 14, maxWidth: 420, mx: "auto", mb: action ? 2 : 0 }}>{hint}</Box>}
      {action}
    </Box>
  );
}

export function ErrorNote({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <Box role="alert" sx={{ background: C.dangerSoft, color: C.danger, borderRadius: "12px", p: 1.5, fontSize: 14, display: "flex", gap: 1.5, alignItems: "center" }}>
      <Box sx={{ flex: 1 }}>{message}</Box>
      {onRetry && <Btn size="sm" variant="danger" onClick={onRetry}>Try again</Btn>}
    </Box>
  );
}

export function Shelf({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <Box component="section" sx={{ mb: 4 }}>
      <Box sx={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", mb: 1.5 }}>
        <Box component="h2" sx={{ m: 0, fontFamily: FONT_DISPLAY, fontSize: 20, fontWeight: 800, letterSpacing: "-0.01em", color: C.ink }}>{title}</Box>
        {action}
      </Box>
      <Box sx={{ display: "flex", gap: 2.2, overflowX: "auto", pb: 1.5, scrollSnapType: "x proximity", "&::-webkit-scrollbar": { height: 6 }, "&::-webkit-scrollbar-thumb": { background: C.line, borderRadius: 3 } }}>
        {children}
      </Box>
    </Box>
  );
}

export function Modal({ title, onClose, children, width = 560, footer }: { title: string; onClose: () => void; children: ReactNode; width?: number; footer?: ReactNode }) {
  useEffect(() => {
    const f = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", f);
    return () => window.removeEventListener("keydown", f);
  }, [onClose]);
  return (
    <Box sx={{ position: "fixed", inset: 0, zIndex: 1400, background: "rgba(22,33,30,.45)", display: "flex", alignItems: "center", justifyContent: "center", p: 2 }} onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <Box role="dialog" aria-modal="true" aria-label={title} sx={{ background: C.surface, color: C.ink, borderRadius: "18px", width: "100%", maxWidth: width, maxHeight: "90vh", display: "flex", flexDirection: "column", boxShadow: "0 20px 60px rgba(0,0,0,.25)" }}>
        <Box sx={{ display: "flex", alignItems: "center", px: 2.5, pt: 2, pb: 1 }}>
          <Box sx={{ flex: 1, fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: 18 }}>{title}</Box>
          <IconBtn label="Close" onClick={onClose} size={34}><CloseIcon fontSize="small" /></IconBtn>
        </Box>
        <Box sx={{ px: 2.5, py: 1, overflowY: "auto", flex: 1 }}>{children}</Box>
        {footer && <Box sx={{ px: 2.5, py: 1.5, display: "flex", gap: 1, justifyContent: "flex-end", borderTop: `1px solid ${C.line}` }}>{footer}</Box>}
      </Box>
    </Box>
  );
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <Box component="label" sx={{ display: "block", mb: 1.6 }}>
      <Box sx={{ fontSize: 13, fontWeight: 700, color: C.ink, mb: 0.5 }}>{label}</Box>
      {children}
      {hint && <Box sx={{ fontSize: 12.5, color: C.muted, mt: 0.4 }}>{hint}</Box>}
    </Box>
  );
}

export const inputSx = {
  width: "100%", boxSizing: "border-box", padding: "9px 12px", fontSize: 14.5, fontFamily: "inherit", color: C.ink, background: C.surface,
  border: `1px solid ${C.line}`, borderRadius: "10px", outline: "none", "&:focus": { borderColor: C.accent, boxShadow: `0 0 0 3px ${C.accentSoft}` },
} as const;

export function TextInput(p: React.InputHTMLAttributes<HTMLInputElement> & { testId?: string }) {
  const { testId, ...rest } = p;
  return <Box component="input" data-testid={testId} sx={inputSx} {...(rest as object)} />;
}
export function TextArea(p: React.TextareaHTMLAttributes<HTMLTextAreaElement> & { testId?: string }) {
  const { testId, ...rest } = p;
  return <Box component="textarea" data-testid={testId} sx={{ ...inputSx, resize: "vertical", minHeight: 80 }} {...(rest as object)} />;
}
export function Select(p: React.SelectHTMLAttributes<HTMLSelectElement> & { testId?: string }) {
  const { testId, ...rest } = p;
  return <Box component="select" data-testid={testId} sx={inputSx} {...(rest as object)} />;
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <Box component="label" sx={{ display: "flex", alignItems: "center", gap: 1.2, cursor: "pointer", py: 0.5, fontSize: 14 }}>
      <Box
        component="input"
        type="checkbox"
        checked={checked}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.checked)}
        sx={{ width: 18, height: 18, accentColor: C.accent, cursor: "pointer" }}
      />
      {label}
    </Box>
  );
}

// ---- toasts ---------------------------------------------------------------------

interface ToastApi {
  show: (msg: string, kind?: "ok" | "error") => void;
}
const ToastCtx = createContext<ToastApi>({ show: () => undefined });
export const useToast = () => useContext(ToastCtx);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<{ id: number; msg: string; kind: string }[]>([]);
  const show = useCallback((msg: string, kind: "ok" | "error" = "ok") => {
    const id = Date.now() + Math.random();
    setItems((l) => [...l, { id, msg, kind }]);
    setTimeout(() => setItems((l) => l.filter((x) => x.id !== id)), 4200);
  }, []);
  const api = useMemo(() => ({ show }), [show]);
  return (
    <ToastCtx.Provider value={api}>
      {children}
      <Box aria-live="polite" sx={{ position: "fixed", left: "50%", transform: "translateX(-50%)", bottom: 100, zIndex: 1500, display: "flex", flexDirection: "column", gap: 1, alignItems: "center" }}>
        {items.map((t) => (
          <Box key={t.id} role="status" sx={{ background: t.kind === "error" ? C.danger : C.ink, color: "#fff", px: 2, py: 1, borderRadius: "999px", fontSize: 14, boxShadow: "0 6px 20px rgba(0,0,0,.2)", maxWidth: "90vw" }}>
            {t.msg}
          </Box>
        ))}
      </Box>
    </ToastCtx.Provider>
  );
}

export function errMsg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function ago(iso?: string): string {
  if (!iso) return "";
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  if (s < 86400 * 14) return `${Math.floor(s / 86400)} d ago`;
  return new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

export const LANGUAGES: { code: string; label: string }[] = [
  { code: "en", label: "English" },
  { code: "hi", label: "Hindi" },
  { code: "ta", label: "Tamil" },
  { code: "te", label: "Telugu" },
  { code: "kn", label: "Kannada" },
  { code: "ml", label: "Malayalam" },
  { code: "bn", label: "Bengali" },
  { code: "mr", label: "Marathi" },
  { code: "gu", label: "Gujarati" },
  { code: "es", label: "Spanish" },
  { code: "fr", label: "French" },
  { code: "de", label: "German" },
];
