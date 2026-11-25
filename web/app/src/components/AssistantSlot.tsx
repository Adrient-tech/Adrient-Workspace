import { lazy, Suspense, useState, type ComponentType } from "react";
import { Box, Button } from "@mui/joy";
import type { AssistantContext, AssistantTool } from "../lib/ai/types";

/**
 * The single assistant button of an app header plus the right-hand panel it
 * opens. The panel lives in src/lib/ai; import.meta.glob finds it when it is
 * present and yields nothing when it is not, so this builds either way: with
 * no panel, the button is not rendered (no dead controls).
 */
interface PanelProps {
  app: string;
  documentId: string;
  documentTitle?: string;
  tools?: AssistantTool[];
  getContext?: () => AssistantContext | null;
  subscribeContext?: (cb: () => void) => () => void;
  systemHint?: string;
  suggestions?: string[];
  onClose?: () => void;
}
const found = import.meta.glob("../lib/ai/AssistantPanel.tsx") as Record<string, () => Promise<{ default: ComponentType<PanelProps> }>>;
const loader = Object.values(found)[0];
const Panel = loader ? lazy(loader) : null;

export interface AssistantSlotProps {
  app: string;
  documentId?: string;
  documentTitle?: string;
  /** Tools of the app (memoise them). Without tools the panel can still chat. */
  tools?: AssistantTool[];
  getContext?: () => AssistantContext | null;
  subscribeContext?: (cb: () => void) => () => void;
  systemHint?: string;
  suggestions?: string[];
}

export function AssistantSlot({ app, documentId = app, documentTitle, tools, getContext, subscribeContext, systemHint, suggestions }: AssistantSlotProps) {
  const [open, setOpen] = useState(false);
  if (!Panel) return null;
  return (
    <>
      <Button size="sm" variant={open ? "soft" : "outlined"} color="neutral" onClick={() => setOpen((o) => !o)} data-testid="assistant-open" aria-pressed={open}>
        Assistant
      </Button>
      {open && (
        <Box
          data-assistant-slot={app}
          data-testid="assistant-dock"
          sx={{
            position: "fixed", right: 0, bottom: 0, top: { xs: 0, md: 56 }, width: { xs: "100%", md: 380 }, zIndex: 1250,
            boxShadow: "-4px 0 16px rgba(0,0,0,.12)", bgcolor: "background.surface", minHeight: 0,
          }}
        >
          <Suspense fallback={null}>
            <Panel
              app={app}
              documentId={documentId}
              documentTitle={documentTitle}
              tools={tools}
              getContext={getContext}
              subscribeContext={subscribeContext}
              systemHint={systemHint}
              suggestions={suggestions}
              onClose={() => setOpen(false)}
            />
          </Suspense>
        </Box>
      )}
    </>
  );
}
