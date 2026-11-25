import { lazy, Suspense, useState } from "react";
import { Box, Button } from "@mui/joy";
import AutoAwesomeIcon from "@mui/icons-material/AutoAwesome";
import type { AssistantContext, AssistantTool } from "@/lib/ai/types";

const Panel = lazy(() => import("@/lib/ai/AssistantPanel"));

export interface BizAssistantProps {
  app: string;
  /** Shown in the panel header, e.g. "Pipeline". */
  title?: string;
  /** Stable id for the chat history; default is the app name. */
  documentId?: string;
  tools: AssistantTool[];
  suggestions: string[];
  systemHint?: string;
  getContext?: () => AssistantContext | null;
  subscribeContext?: (cb: () => void) => () => void;
}

/**
 * The assistant for a business app: one button in the header and a docked
 * right-hand panel (full width on phones). Built on AssistantPanel; the app
 * supplies its tool set (pages/<app>/ai/tools.ts) and suggestion chips.
 * Place it where AssistantSlot would go.
 */
export function BizAssistant(p: BizAssistantProps) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        size="sm"
        variant={open ? "solid" : "soft"}
        color="primary"
        startDecorator={<AutoAwesomeIcon fontSize="small" />}
        onClick={() => setOpen((o) => !o)}
        data-testid="assistant-open"
        aria-pressed={open}
      >
        Assistant
      </Button>
      {open && (
        <Box
          data-assistant-slot={p.app}
          sx={{
            position: "fixed",
            top: { xs: 0, sm: 65 },
            right: 0,
            bottom: 0,
            width: { xs: "100%", sm: 400 },
            zIndex: 1250,
            boxShadow: "-8px 0 24px rgba(0,0,0,.12)",
            minHeight: 0,
          }}
        >
          <Suspense fallback={null}>
            <Panel
              app={p.app}
              documentId={p.documentId ?? p.app}
              documentTitle={p.title}
              tools={p.tools}
              suggestions={p.suggestions}
              systemHint={p.systemHint}
              getContext={p.getContext}
              subscribeContext={p.subscribeContext}
              onClose={() => setOpen(false)}
            />
          </Suspense>
        </Box>
      )}
    </>
  );
}
