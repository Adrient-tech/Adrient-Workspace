// Responsive ribbon engine shared by Sheets, Docs and Slides.
//
// Like Excel, a ribbon panel shrinks in steps instead of scrolling sideways:
//   step 0   everything at full size (large icons with labels)
//   then     one action per step (see shrinkActions): ordinary groups go small,
//            minor groups fold into a drop-down button that opens the group in
//            a popover, then key groups go small, then the rest fold.
// Groups with a lower `priority` number shrink first; ties go right to left,
// so the most used groups keep their large icons the longest.
// The engine measures real widths: after each render it checks whether the
// panel overflows and applies one more step, before the browser paints.
import { createContext, useContext, useId, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Box, Typography } from "@mui/joy";
import { Popper } from "@mui/base/Popper";
import { ClickAwayListener } from "@mui/base/ClickAwayListener";
import ViewModuleIcon from "@mui/icons-material/ViewModule";

interface Ctx {
  /** Buttons should render small (no labels) at this step. */
  compact: boolean;
  /** Group ids whose buttons are small at this step. */
  compactGroups: ReadonlySet<string>;
  /** Group ids that are collapsed into a dropdown at this step. */
  collapsed: ReadonlySet<string>;
  register: (id: string, priority: number) => () => void;
}

const RibbonCtx = createContext<Ctx | null>(null);

/** True when the surrounding ribbon is in its compact (small icon) step. */
export function useRibbonCompact(): boolean {
  return useContext(RibbonCtx)?.compact ?? false;
}

/** Pure: ids in the order they collapse. Lower priority first, then right to left. */
export function collapseOrder(items: { id: string; priority: number; index: number }[]): string[] {
  return [...items].sort((a, b) => a.priority - b.priority || b.index - a.index).map((i) => i.id);
}

/** One shrink action: show a group small, or fold it into a single drop-down button. */
export interface ShrinkAction {
  id: string;
  kind: "compact" | "collapse";
}

/** Groups at or above this priority keep full size until the minor groups have collapsed. */
export const KEEP_PRIORITY = 70;
/** Groups below this priority are minor: they collapse before key groups lose their labels. */
export const MINOR_PRIORITY = 40;

/**
 * Pure: the order of shrink actions, one per step, like Excel. Ordinary groups
 * go small first (lowest priority first), then minor groups fold into drop-downs,
 * then key groups (Clipboard, Font, Editing …) go small, then the rest fold.
 * `order` is from collapseOrder; `prio` gives each id's priority.
 */
export function shrinkActions(order: string[], prio: (id: string) => number): ShrinkAction[] {
  const keep = order.filter((id) => prio(id) >= KEEP_PRIORITY);
  const rest = order.filter((id) => prio(id) < KEEP_PRIORITY);
  const minor = rest.filter((id) => prio(id) < MINOR_PRIORITY);
  const out: ShrinkAction[] = [
    ...rest.map((id) => ({ id, kind: "compact" as const })),
    ...minor.map((id) => ({ id, kind: "collapse" as const })),
    ...keep.map((id) => ({ id, kind: "compact" as const })),
  ];
  for (const id of order) if (!minor.includes(id)) out.push({ id, kind: "collapse" });
  return out;
}

/** Pure: the groups shown small at `step`. */
export function compactAtStep(actions: ShrinkAction[], step: number): Set<string> {
  return new Set(actions.slice(0, Math.max(0, step)).filter((a) => a.kind === "compact").map((a) => a.id));
}

/** Pure: the groups folded into a drop-down at `step`. */
export function collapsedAtStep(actions: ShrinkAction[], step: number): Set<string> {
  return new Set(actions.slice(0, Math.max(0, step)).filter((a) => a.kind === "collapse").map((a) => a.id));
}

/**
 * Wrap the groups of one ribbon panel. `resetKey` must change when the panel
 * content changes (for example the active tab) so the fit is measured again.
 */
export function ResponsiveRibbonBody({ children, resetKey, sx, role = "tabpanel", testId, scrollerRef, onKeyDown, rowProps }: { children: ReactNode; resetKey?: string; sx?: object; role?: string; testId?: string; scrollerRef?: { current: HTMLDivElement | null }; onKeyDown?: (e: React.KeyboardEvent) => void; rowProps?: Record<string, unknown> }) {
  const hostRef = useRef<HTMLDivElement>(null);
  const rowRef = useRef<HTMLDivElement>(null);
  const reg = useRef(new Map<string, number>());
  const order = useRef<string[]>([]);
  const actions = useRef<ShrinkAction[]>([]);
  const [step, setStep] = useState(0);
  const [width, setWidth] = useState(0);
  const [measured, setMeasured] = useState(-1);

  // A new tab or a new width starts again from the full-size layout.
  useLayoutEffect(() => {
    setStep(0);
    setMeasured(-1);
  }, [resetKey, width]);

  useLayoutEffect(() => {
    const el = hostRef.current;
    if (!el) return;
    setWidth(el.clientWidth);
    // jsdom and very old browsers have no ResizeObserver: measure once and keep that layout.
    if (typeof ResizeObserver === "undefined") return;
    let raf = 0;
    const ro = new ResizeObserver(() => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => setWidth((w) => (Math.abs(w - el.clientWidth) > 1 ? el.clientWidth : w)));
    });
    ro.observe(el);
    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
    };
  }, []);

  // After every render: refresh the collapse order from the DOM, then add one step while the row overflows.
  useLayoutEffect(() => {
    const row = rowRef.current;
    if (!row) return;
    if (scrollerRef) scrollerRef.current = row;
    const nodes = Array.from(row.querySelectorAll<HTMLElement>("[data-rgroup]"));
    order.current = collapseOrder(
      nodes.map((n, index) => ({ id: n.dataset.rgroup as string, priority: reg.current.get(n.dataset.rgroup as string) ?? 50, index })),
    );
    actions.current = shrinkActions(order.current, (id) => reg.current.get(id) ?? 50);
    const maxStep = actions.current.length;
    const overflow = row.scrollWidth > row.clientWidth + 1;
    if (overflow && step < maxStep) setStep(step + 1);
    else if (measured !== step) setMeasured(step);
  });

  const ctx = useMemo<Ctx>(
    () => ({
      compact: false,
      compactGroups: compactAtStep(actions.current, step),
      collapsed: collapsedAtStep(actions.current, step),
      register: (id, priority) => {
        reg.current.set(id, priority);
        return () => {
          reg.current.delete(id);
        };
      },
    }),
    // order.current is refreshed in the layout effect above, right before the step changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [step, measured],
  );

  return (
    <Box ref={hostRef} sx={{ position: "relative", minWidth: 0 }}>
      <RibbonCtx.Provider value={ctx}>
        <Box
          ref={rowRef}
          role={role}
          onKeyDown={onKeyDown}
          {...rowProps}
          data-testid={testId}
          data-ribbon-step={step}
          sx={{ display: "flex", alignItems: "stretch", overflowX: "auto", scrollbarWidth: "none", "&::-webkit-scrollbar": { display: "none" }, ...sx }}
        >
          {children}
        </Box>
      </RibbonCtx.Provider>
    </Box>
  );
}

/**
 * One ribbon group. Renders `children` inline, or, when the ribbon is too narrow,
 * as a single button (icon + title) that opens the same content in a popover.
 */
export function ResponsiveGroup({
  title,
  icon,
  priority = 50,
  children,
  maxWidth = 330,
  sx,
}: {
  title: string;
  icon?: ReactNode;
  /** Lower collapses earlier. Default 50. */
  priority?: number;
  children: ReactNode;
  maxWidth?: number;
  sx?: object;
}) {
  const ctx = useContext(RibbonCtx);
  const id = useId();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  useLayoutEffect(() => ctx?.register(id, priority), [ctx, id, priority]);
  const isCollapsed = !!ctx && ctx.collapsed.has(id);
  const frame = { display: "flex", flexDirection: "column" as const, px: 0.75, borderRight: "1px solid", borderColor: "divider", flexShrink: 0, ...sx };

  const groupCtx = useMemo<Ctx | null>(() => (ctx ? { ...ctx, compact: ctx.compactGroups.has(id) } : null), [ctx, id]);
  if (!isCollapsed) {
    return (
      <Box role="group" aria-label={title} data-rgroup={id} data-compact={groupCtx?.compact ? "1" : undefined} sx={frame}>
        <RibbonCtx.Provider value={groupCtx}>
          <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 0.25, flex: 1, maxWidth }}>{children}</Box>
        </RibbonCtx.Provider>
        <Typography level="body-xs" sx={{ textAlign: "center", opacity: 0.6, fontSize: 10 }}>
          {title}
        </Typography>
      </Box>
    );
  }
  const open = !!anchor;
  return (
    <Box role="group" aria-label={title} data-rgroup={id} data-collapsed="1" sx={frame}>
      <Box sx={{ display: "flex", alignItems: "center", flex: 1 }}>
        <ClickAwayListener onClickAway={() => setAnchor(null)}>
          <Box onKeyDown={(e: React.KeyboardEvent) => e.key === "Escape" && setAnchor(null)}>
            <Box
              component="button"
              type="button"
              aria-label={`${title} group`}
              aria-haspopup="true"
              aria-expanded={open}
              data-rb="1"
              data-testid={`ribbon-group-${title.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`}
              onMouseDown={(e: React.MouseEvent) => e.preventDefault()}
              onClick={(e: React.MouseEvent<HTMLElement>) => setAnchor(open ? null : e.currentTarget)}
              sx={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                gap: 0.25,
                minWidth: 56,
                height: 54,
                px: 0.5,
                border: "none",
                borderRadius: "4px",
                bgcolor: open ? "background.level2" : "transparent",
                color: "text.primary",
                font: "inherit",
                cursor: "pointer",
                "&:hover": { bgcolor: "background.level1" },
                "& svg": { fontSize: 22 },
              }}
            >
              {icon ?? <ViewModuleIcon />}
              <Typography level="body-xs" sx={{ lineHeight: 1.1, whiteSpace: "nowrap" }}>
                {title} ▾
              </Typography>
            </Box>
            <Popper open={open} anchorEl={anchor} placement="bottom-start" style={{ zIndex: 1500 }}>
              <RibbonCtx.Provider value={POPOVER_CTX}>
                <Box
                  role="dialog"
                  aria-label={title}
                  sx={{
                    mt: 0.5,
                    p: 1,
                    display: "flex",
                    alignItems: "center",
                    flexWrap: "wrap",
                    gap: 0.5,
                    maxWidth: "min(560px, 94vw)",
                    bgcolor: "background.popup",
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: "8px",
                    boxShadow: "lg",
                  }}
                >
                  {children}
                </Box>
              </RibbonCtx.Provider>
            </Popper>
          </Box>
        </ClickAwayListener>
      </Box>
    </Box>
  );
}

// Inside a popover everything is shown at full size and never collapses again.
const POPOVER_CTX: Ctx = { compact: false, compactGroups: new Set(), collapsed: new Set(), register: () => () => undefined };
