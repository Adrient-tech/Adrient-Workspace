import type { ReactNode } from "react";
import { Box, Typography } from "@mui/joy";

export type Illustration = "files" | "inbox" | "tasks" | "calendar" | "approvals" | "search" | "generic";

/**
 * Small inline-SVG illustrations for empty states. Two tones from the
 * Adrient palette (green + saffron) over a soft disc; they read on light and
 * dark surfaces because every fill uses translucent colour over currentColor.
 */
export function Illustration({ name, size = 88 }: { name: Illustration; size?: number }) {
  const body = (() => {
    switch (name) {
      case "files":
        return (
          <>
            <path d="M22 36a5 5 0 0 1 5-5h14l5 6h27a5 5 0 0 1 5 5v26a5 5 0 0 1-5 5H27a5 5 0 0 1-5-5z" fill="var(--ad-green)" fillOpacity=".16" stroke="var(--ad-green)" strokeWidth="2.4" strokeLinejoin="round" />
            <rect x="34" y="50" width="30" height="5" rx="2.5" fill="var(--ad-saffron)" />
          </>
        );
      case "inbox":
        return (
          <>
            <path d="M20 50l9-18a5 5 0 0 1 4.5-2.8h29a5 5 0 0 1 4.5 2.8l9 18v18a5 5 0 0 1-5 5H25a5 5 0 0 1-5-5z" fill="var(--ad-green)" fillOpacity=".16" stroke="var(--ad-green)" strokeWidth="2.4" strokeLinejoin="round" />
            <path d="M20 50h20c0 5 4 8 10 8s10-3 10-8h20" fill="none" stroke="var(--ad-green)" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
            <circle cx="70" cy="26" r="6" fill="var(--ad-saffron)" />
          </>
        );
      case "tasks":
        return (
          <>
            <rect x="24" y="22" width="52" height="56" rx="9" fill="var(--ad-green)" fillOpacity=".16" stroke="var(--ad-green)" strokeWidth="2.4" />
            <path d="M34 40l4 4 8-9M34 60l4 4 8-9" fill="none" stroke="var(--ad-green)" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" />
            <rect x="52" y="38" width="16" height="4.5" rx="2.2" fill="var(--ad-saffron)" />
            <rect x="52" y="58" width="12" height="4.5" rx="2.2" fill="var(--ad-green)" fillOpacity=".4" />
          </>
        );
      case "calendar":
        return (
          <>
            <rect x="22" y="26" width="56" height="50" rx="10" fill="var(--ad-green)" fillOpacity=".16" stroke="var(--ad-green)" strokeWidth="2.4" />
            <path d="M22 42h56M36 20v12M64 20v12" stroke="var(--ad-green)" strokeWidth="2.4" strokeLinecap="round" />
            <rect x="32" y="52" width="12" height="12" rx="3.5" fill="var(--ad-saffron)" />
            <rect x="52" y="52" width="12" height="12" rx="3.5" fill="var(--ad-green)" fillOpacity=".35" />
          </>
        );
      case "approvals":
        return (
          <>
            <path d="M50 20l26 10v18c0 16-11 26-26 32-15-6-26-16-26-32V30z" fill="var(--ad-green)" fillOpacity=".16" stroke="var(--ad-green)" strokeWidth="2.4" strokeLinejoin="round" />
            <path d="M38 50l8 8 16-17" fill="none" stroke="var(--ad-saffron)" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" />
          </>
        );
      case "search":
        return (
          <>
            <circle cx="44" cy="44" r="19" fill="var(--ad-green)" fillOpacity=".16" stroke="var(--ad-green)" strokeWidth="2.6" />
            <path d="M58 58l17 17" stroke="var(--ad-saffron)" strokeWidth="5" strokeLinecap="round" />
          </>
        );
      default:
        return (
          <>
            <rect x="26" y="26" width="48" height="48" rx="14" fill="var(--ad-green)" fillOpacity=".16" stroke="var(--ad-green)" strokeWidth="2.4" />
            <circle cx="50" cy="50" r="8" fill="var(--ad-saffron)" />
          </>
        );
    }
  })();
  return (
    <svg width={size} height={size} viewBox="0 0 100 100" role="img" aria-hidden focusable="false">
      <circle cx="50" cy="52" r="42" fill="currentColor" fillOpacity=".05" />
      {body}
    </svg>
  );
}

interface EmptyStateProps {
  illustration?: Illustration;
  title: string;
  hint?: ReactNode;
  action?: ReactNode;
  /** Compact variant for cards (smaller art, tighter spacing). */
  compact?: boolean;
}

/** EmptyState is the one empty/zero-data pattern: small illustration, a short
 *  title, one line of guidance and at most one action. */
export function EmptyState({ illustration = "generic", title, hint, action, compact }: EmptyStateProps) {
  return (
    <Box
      data-testid="empty-state"
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        textAlign: "center",
        gap: compact ? 0.5 : 1,
        py: compact ? 2 : 6,
        px: 2,
        color: "text.tertiary",
      }}
    >
      <Illustration name={illustration} size={compact ? 64 : 96} />
      <Typography level={compact ? "title-sm" : "title-md"} sx={{ color: "text.primary" }}>
        {title}
      </Typography>
      {hint && (
        <Typography level="body-sm" sx={{ maxWidth: 320 }}>
          {hint}
        </Typography>
      )}
      {action && <Box sx={{ mt: 1 }}>{action}</Box>}
    </Box>
  );
}
