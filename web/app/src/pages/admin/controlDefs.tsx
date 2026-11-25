import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { controlsApi, type ControlDef } from "./controlsApi";

const Ctx = createContext<ControlDef[]>([]);

/** Loads the control catalog once so value labels render with their definition. */
export function ControlDefsProvider({ children }: { children: ReactNode }) {
  const [defs, setDefs] = useState<ControlDef[]>([]);
  useEffect(() => {
    let alive = true;
    controlsApi
      .list()
      .then((r) => alive && setDefs(r.catalog))
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);
  return <Ctx.Provider value={defs}>{children}</Ctx.Provider>;
}

export function useDef(key: string): ControlDef | undefined {
  return useContext(Ctx).find((d) => d.key === key);
}
