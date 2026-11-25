/* eslint-disable @typescript-eslint/no-explicit-any -- FortuneSheet ref. */
import { lazy, Suspense, useCallback, useMemo, useRef } from "react";
import { Box } from "@mui/joy";
import type { ChartConfig } from "../chartData";
import { fortuneAdapter, fortuneSelection } from "./fortuneAdapter";
import { createSheetTools, sheetContext } from "./tools";

const AssistantPanel = lazy(() => import("@/lib/ai/AssistantPanel"));

export interface SheetsAssistantProps {
  sheetId: string;
  title: string;
  getWb: () => any;
  upsertChart: (cfg: ChartConfig) => void;
  removeChart: (id: string) => void;
  onClose: () => void;
}

/** Right-hand AI Assistant panel for the spreadsheet editor (full screen on phones). */
export default function SheetsAssistant(p: SheetsAssistantProps) {
  const { getWb } = p;
  // The chart callbacks change every editor render; read the latest through a ref.
  const latest = useRef(p);
  latest.current = p;
  const adapter = useMemo(
    () => fortuneAdapter({ getWb, upsertChart: (c) => latest.current.upsertChart(c), removeChart: (id) => latest.current.removeChart(id) }),
    [getWb],
  );
  const tools = useMemo(() => createSheetTools(adapter), [adapter]);
  const getContext = useCallback(() => {
    try {
      return sheetContext(adapter, fortuneSelection(getWb));
    } catch {
      return null;
    }
  }, [adapter, getWb]);
  const subscribeContext = useCallback((cb: () => void) => {
    const on = () => window.setTimeout(cb, 0);
    window.addEventListener("mouseup", on);
    window.addEventListener("keyup", on);
    return () => {
      window.removeEventListener("mouseup", on);
      window.removeEventListener("keyup", on);
    };
  }, []);
  return (
    <Box
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
        <AssistantPanel
          app="sheets"
          documentId={p.sheetId}
          documentTitle={p.title}
          tools={tools}
          getContext={getContext}
          subscribeContext={subscribeContext}
          onClose={p.onClose}
        />
      </Suspense>
    </Box>
  );
}
