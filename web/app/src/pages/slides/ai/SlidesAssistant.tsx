import { lazy, Suspense, useCallback, useMemo, useRef } from "react";
import { Box } from "@mui/joy";
import type { Slide } from "../model";
import { deckAdapter } from "./deckAdapter";
import { createSlideTools, slideContext } from "./tools";

const AssistantPanel = lazy(() => import("@/lib/ai/AssistantPanel"));

export interface SlidesAssistantProps {
  deckId: string;
  title: string;
  getSlides: () => Slide[];
  setSlides: (s: Slide[]) => void;
  getCurrent: () => number;
  onClose: () => void;
}

/** Right-hand AI Assistant for the deck editor (full screen on phones). */
export default function SlidesAssistant(p: SlidesAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const adapter = useMemo(
    () =>
      deckAdapter({
        getSlides: () => latest.current.getSlides(),
        setSlides: (s) => latest.current.setSlides(s),
        getCurrent: () => latest.current.getCurrent(),
      }),
    [],
  );
  const tools = useMemo(() => createSlideTools(adapter), [adapter]);
  const getContext = useCallback(() => {
    try {
      return slideContext(adapter);
    } catch {
      return null;
    }
  }, [adapter]);
  return (
    <Box
      data-testid="assistant-panel"
      sx={{
        width: { xs: "100%", md: 340 },
        flexShrink: 0,
        position: { xs: "fixed", md: "relative" },
        inset: { xs: 0, md: "auto" },
        zIndex: { xs: 1300, md: "auto" },
        minHeight: 0,
      }}
    >
      <Suspense fallback={null}>
        <AssistantPanel app="slides" documentId={p.deckId} documentTitle={p.title} tools={tools} getContext={getContext} onClose={p.onClose} />
      </Suspense>
    </Box>
  );
}
