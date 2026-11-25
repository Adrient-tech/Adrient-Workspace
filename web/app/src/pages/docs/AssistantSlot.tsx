// Right-hand slot for the AI Assistant. When the shared assistant panel
// (src/lib/ai/AssistantPanel) exists it is loaded lazily and given the
// editor as context; until then the slot says plainly that it is not
// connected. The glob keeps the build working whether or not the file exists.
import { lazy, Suspense, type ComponentType } from "react";
import { Box, IconButton, Typography } from "@mui/joy";
import CloseIcon from "@mui/icons-material/Close";
import type { Editor } from "@tiptap/react";

export interface AssistantContext {
  app: "docs";
  title: string;
  editor: Editor | null;
}
interface PanelProps {
  context: AssistantContext;
  onClose: () => void;
}

const modules = import.meta.glob("../../lib/ai/AssistantPanel.tsx") as Record<string, () => Promise<{ default: ComponentType<PanelProps> }>>;
const loader = Object.values(modules)[0];
export const hasAssistant = !!loader;
const Panel = loader ? lazy(loader) : null;

export function AssistantSlot({ editor, title, onClose }: { editor: Editor | null; title: string; onClose: () => void }) {
  return (
    <Box
      data-testid="assistant-slot"
      sx={{ width: 340, flexShrink: 0, borderLeft: "1px solid", borderColor: "divider", bgcolor: "background.surface", display: "flex", flexDirection: "column" }}
    >
      {Panel ? (
        <Suspense fallback={<Typography level="body-sm" sx={{ p: 2 }}>Loading assistant…</Typography>}>
          <Panel context={{ app: "docs", title, editor }} onClose={onClose} />
        </Suspense>
      ) : (
        <>
          <Box sx={{ display: "flex", alignItems: "center", p: 1.5 }}>
            <Typography level="title-sm" sx={{ flex: 1 }}>AI Assistant</Typography>
            <IconButton size="sm" variant="plain" aria-label="Close assistant" onClick={onClose}>
              <CloseIcon />
            </IconButton>
          </Box>
          <Typography level="body-sm" sx={{ px: 1.5, color: "text.tertiary" }}>
            The AI Assistant is not connected in this build. When it is, it appears here and can read and edit this document.
          </Typography>
        </>
      )}
    </Box>
  );
}
