import { useCallback, useMemo, useRef } from "react";
import { AssistantSlot } from "../../../components/AssistantSlot";
import type { CalendarEvent } from "../types";
import { calendarAdapter } from "./adapter";
import { CALENDAR_SUGGESTIONS, calendarContext, createCalendarTools, type CalEvent } from "./tools";

export interface CalendarAssistantProps {
  range: { from: Date; to: Date };
  label: string;
  events: CalendarEvent[];
  reload: () => void;
}

/** The Assistant button and right-hand panel of Calendar. */
export function CalendarAssistant(p: CalendarAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const tools = useMemo(
    () =>
      createCalendarTools(
        calendarAdapter({
          visibleRange: () => ({ from: latest.current.range.from, to: latest.current.range.to, label: latest.current.label }),
          changed: () => latest.current.reload(),
        }),
      ),
    [],
  );
  const getContext = useCallback(() => calendarContext(latest.current.label, latest.current.events as CalEvent[]), []);
  return <AssistantSlot app="calendar" documentId="calendar" documentTitle="Calendar" tools={tools} getContext={getContext} suggestions={CALENDAR_SUGGESTIONS} />;
}
