import { lazy, Suspense, useCallback, useEffect, useMemo, useRef } from "react";
import { Box } from "@mui/joy";
import type { Editor } from "@tiptap/react";
import { tiptapAdapter } from "./tiptapAdapter";
import { createDocTools, docContext } from "./tools";

const AssistantPanel = lazy(() => import("@/lib/ai/AssistantPanel"));

/** Right-hand AI Assistant for the document editor (full screen on phones). */
export default function DocsAssistant({ editor, docId, title, onClose }: { editor: Editor | null; docId: string; title: string; onClose: () => void }) {
  const ed = useRef(editor);
  ed.current = editor;
  const titleRef = useRef(title);
  titleRef.current = title;
  const adapter = useMemo(() => tiptapAdapter(() => ed.current), []);
  const tools = useMemo(() => createDocTools(adapter), [adapter]);
  const getContext = useCallback(() => {
    try {
      return docContext(adapter, titleRef.current);
    } catch {
      return null;
    }
  }, [adapter]);
  const subscribers = useRef(new Set<() => void>());
  useEffect(() => {
    if (!editor) return;
    const fire = () => subscribers.current.forEach((f) => f());
    editor.on("selectionUpdate", fire);
    return () => {
      editor.off("selectionUpdate", fire);
    };
  }, [editor]);
  const subscribeContext = useCallback((cb: () => void) => {
    subscribers.current.add(cb);
    return () => {
      subscribers.current.delete(cb);
    };
  }, []);
  return (
    <Box
      data-testid="assistant-slot"
      sx={{
        width: { xs: "100%", md: 360 },
        flexShrink: 0,
        position: { xs: "fixed", md: "relative" },
        inset: { xs: 0, md: "auto" },
        zIndex: { xs: 1300, md: "auto" },
        minHeight: 0,
      }}
    >
      <Suspense fallback={null}>
        <AssistantPanel app="docs" documentId={docId} documentTitle={title} tools={tools} getContext={getContext} subscribeContext={subscribeContext} onClose={onClose} />
      </Suspense>
    </Box>
  );
}
