import { useEffect, useState } from "react";
import { Box, Typography } from "@mui/joy";
import { findFunction, type FunctionInfo } from "./functionCatalog";
import { activeCallAt, hintArgIndex } from "./formulaHint";

// Function signature tooltip while a formula is typed, in the cell editor or
// the formula bar: shows NAME(arg1, [arg2]) with the current argument in bold
// and its help below. Read-only: it never touches the edit.

const EDITOR_SEL = ".fortune-fx-input, .luckysheet-cell-input, .luckysheet-input-box-inner";

function caretOffset(el: HTMLElement): number {
  const s = window.getSelection();
  if (!s || s.rangeCount === 0 || !el.contains(s.anchorNode)) return (el.textContent ?? "").length;
  const r = s.getRangeAt(0).cloneRange();
  r.selectNodeContents(el);
  r.setEnd(s.anchorNode as Node, s.anchorOffset);
  return r.toString().length;
}

interface Hint {
  fn: FunctionInfo;
  argIndex: number;
  x: number;
  y: number;
}

export function FormulaHint() {
  const [hint, setHint] = useState<Hint | null>(null);

  useEffect(() => {
    const update = () => {
      const a = document.activeElement as HTMLElement | null;
      const ed = a?.closest?.(EDITOR_SEL) as HTMLElement | null;
      if (!ed || !ed.isContentEditable) return setHint(null);
      const text = ed.textContent ?? "";
      const call = activeCallAt(text, caretOffset(ed));
      const fn = call && findFunction(call.name);
      if (!call || !fn) return setHint(null);
      const box = ed.getBoundingClientRect();
      setHint({ fn, argIndex: call.argIndex, x: Math.max(8, box.left), y: box.bottom + 4 });
    };
    const later = () => window.setTimeout(update, 0);
    document.addEventListener("input", later, true);
    document.addEventListener("keyup", later, true);
    document.addEventListener("mouseup", later, true);
    document.addEventListener("focusin", later, true);
    document.addEventListener("focusout", later, true);
    return () => {
      document.removeEventListener("input", later, true);
      document.removeEventListener("keyup", later, true);
      document.removeEventListener("mouseup", later, true);
      document.removeEventListener("focusin", later, true);
      document.removeEventListener("focusout", later, true);
    };
  }, []);

  if (!hint) return null;
  const { fn } = hint;
  const cur = hintArgIndex(fn.args, hint.argIndex);
  return (
    <Box
      role="tooltip"
      data-testid="formula-hint"
      sx={{
        position: "fixed",
        left: hint.x,
        top: hint.y,
        zIndex: 2000,
        maxWidth: 420,
        bgcolor: "background.surface",
        border: "1px solid",
        borderColor: "neutral.outlinedBorder",
        borderLeft: "3px solid #126C5A",
        borderRadius: "8px",
        boxShadow: "md",
        px: 1.25,
        py: 0.75,
        pointerEvents: "none",
      }}
    >
      <Typography level="body-sm" sx={{ fontFamily: "monospace", color: "text.primary" }}>
        {fn.name}(
        {fn.args.map((a, i) => (
          <span key={i}>
            {i > 0 && ", "}
            <span style={i === cur ? { fontWeight: 700, color: "#126C5A", textDecoration: "underline" } : { color: "#5b6b66" }}>
              {a.optional ? `[${a.name}]` : a.name}
              {a.repeat ? "…" : ""}
            </span>
          </span>
        ))}
        )
      </Typography>
      <Typography level="body-xs" sx={{ mt: 0.25 }}>
        {cur >= 0 && fn.args[cur]?.help ? `${fn.args[cur].name}: ${fn.args[cur].help}` : fn.description}
      </Typography>
    </Box>
  );
}
