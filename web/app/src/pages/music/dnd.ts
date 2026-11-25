import { useRef, useState, type DragEvent } from "react";

/** useDragReorder wires HTML5 drag and drop onto a list: spread
 *  `rowProps(i)` on each row; `onMove(from, to)` is called on drop. `over` is
 *  the row currently hovered (for a drop indicator). */
export function useDragReorder(onMove: (from: number, to: number) => void) {
  const from = useRef<number | null>(null);
  const [over, setOver] = useState<number | null>(null);
  const [dragging, setDragging] = useState<number | null>(null);
  return {
    over,
    dragging,
    rowProps: (i: number) => ({
      draggable: true,
      onDragStart: (e: DragEvent) => {
        from.current = i;
        setDragging(i);
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData("text/plain", String(i));
      },
      onDragOver: (e: DragEvent) => {
        if (from.current === null) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        if (over !== i) setOver(i);
      },
      onDragLeave: () => setOver((o) => (o === i ? null : o)),
      onDrop: (e: DragEvent) => {
        e.preventDefault();
        const f = from.current;
        from.current = null;
        setOver(null);
        setDragging(null);
        if (f !== null && f !== i) onMove(f, i);
      },
      onDragEnd: () => {
        from.current = null;
        setOver(null);
        setDragging(null);
      },
    }),
  };
}
