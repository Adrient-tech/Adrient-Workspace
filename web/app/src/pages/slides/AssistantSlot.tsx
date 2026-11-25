import { Box } from "@mui/joy";

/** Right-hand panel slot for the AI assistant. Empty (and hidden) until the
 *  assistant panel module (lib/ai/AssistantPanel) exists in the build. */
export function AssistantSlot() {
  return <Box data-testid="assistant-slot" sx={{ display: "none" }} />;
}
