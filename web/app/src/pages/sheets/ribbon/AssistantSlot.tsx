/* eslint-disable @typescript-eslint/no-explicit-any -- the panel component is provided by another module. */
import { Suspense, lazy, type ComponentType } from "react";
import { Box } from "@mui/joy";

// The AI Assistant panel is built elsewhere (web/app/src/lib/ai/AssistantPanel).
// When that module is not on this branch, the glob is empty and the slot renders
// nothing (no placeholder, no fake panel).
const found = import.meta.glob("../../../lib/ai/AssistantPanel.tsx");
const loader = Object.values(found)[0] as (() => Promise<{ default?: ComponentType<any>; AssistantPanel?: ComponentType<any> }>) | undefined;
const Panel = loader
  ? lazy(async () => {
      const m = await loader();
      return { default: (m.default ?? m.AssistantPanel) as ComponentType<any> };
    })
  : null;

export const hasAssistant = Panel !== null;

export function AssistantSlot({ open, onClose, documentId, getWb }: { open: boolean; onClose: () => void; documentId: string; getWb: () => any }) {
  if (!Panel || !open) return null;
  return (
    <Box data-testid="assistant-slot" sx={{ width: 360, flexShrink: 0, borderLeft: "1px solid", borderColor: "divider", overflow: "auto" }}>
      <Suspense fallback={null}>
        <Panel app="sheets" documentId={documentId} getWb={getWb} onClose={onClose} />
      </Suspense>
    </Box>
  );
}
