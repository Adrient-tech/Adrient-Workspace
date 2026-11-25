import { useCallback, useEffect, useMemo, useState } from "react";
import { BizAssistant } from "../../components/BizAssistant";
import { usePermissions } from "../../lib/permissions";
import { createPosTools, POS_HINT, POS_SUGGESTIONS } from "./ai/tools";
import { Box, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import PointOfSaleIcon from "@mui/icons-material/PointOfSale";
import ReceiptLongOutlined from "@mui/icons-material/ReceiptLongOutlined";
import ShoppingBagOutlined from "@mui/icons-material/ShoppingBagOutlined";
import SavingsOutlined from "@mui/icons-material/SavingsOutlined";
import BarChartIcon from "@mui/icons-material/BarChart";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import { getSettings, listDiscountPresets, listRegisters, listTaxRates } from "./api";
import { RegisterView } from "./RegisterView";
import { ReportsView } from "./ReportsView";
import { SalesView } from "./SalesView";
import { SettingsView } from "./SettingsView";
import { AdvancedSettings } from "./AdvancedSettings";
import { ShiftsView } from "./ShiftsView";
import type { PosDiscountPreset, PosRegister, PosSettings, PosTaxRate } from "./types";
import { ACCENT } from "./ui";

type View = "register" | "sales" | "drawer" | "reports" | "settings";

const NAV: { id: View; label: string; icon: JSX.Element }[] = [
  { id: "register", label: "Register", icon: <ShoppingBagOutlined /> },
  { id: "sales", label: "Sales", icon: <ReceiptLongOutlined /> },
  { id: "drawer", label: "Cash drawer", icon: <SavingsOutlined /> },
  { id: "reports", label: "Reports", icon: <BarChartIcon /> },
  { id: "settings", label: "Settings", icon: <SettingsOutlined /> },
];

const DEFAULT_SETTINGS: PosSettings = {
  store_name: "",
  currency: "",
  receipt_header: "",
  receipt_footer: "Thank you!",
  loyalty_earn_per_unit: 0,
  loyalty_point_value: 0.01,
};

const REG_KEY = "adrient.pos.register";

export default function PosApp({ user }: { user: User }) {
  const [view, setView] = useState<View>("register");
  const [registers, setRegisters] = useState<PosRegister[]>([]);
  const [taxes, setTaxes] = useState<PosTaxRate[]>([]);
  const [presets, setPresets] = useState<PosDiscountPreset[]>([]);
  const [settings, setSettings] = useState<PosSettings>(DEFAULT_SETTINGS);
  const [registerId, setRegisterIdState] = useState(() => localStorage.getItem(REG_KEY) ?? "");
  const [version, setVersion] = useState(0);
  const [loaded, setLoaded] = useState(false);

  const reload = useCallback(async () => {
    const [r, t, p, s] = await Promise.all([
      listRegisters().catch(() => [] as PosRegister[]),
      listTaxRates().catch(() => [] as PosTaxRate[]),
      listDiscountPresets().catch(() => [] as PosDiscountPreset[]),
      getSettings().catch(() => DEFAULT_SETTINGS),
    ]);
    setRegisters(r);
    setTaxes(t);
    setPresets(p);
    setSettings({ ...DEFAULT_SETTINGS, ...s });
    setLoaded(true);
  }, []);
  useEffect(() => {
    reload();
  }, [reload, version]);

  // Keep a valid register selected.
  useEffect(() => {
    if (!registers.length) return;
    if (!registers.some((r) => r.id === registerId)) {
      setRegisterIdState(registers[0].id);
    }
  }, [registers, registerId]);

  const setRegisterId = (id: string) => {
    localStorage.setItem(REG_KEY, id);
    setRegisterIdState(id);
  };
  const changed = useCallback(() => setVersion((v) => v + 1), []);
  const { can: canDo } = usePermissions();
  const aiTools = useMemo(() => createPosTools(), []);

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <PointOfSaleIcon sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>
            POS
          </Typography>
          {canDo("pos:read") && (
            <Box sx={{ ml: "auto" }}>
              <BizAssistant app="pos" title="POS" tools={aiTools} suggestions={POS_SUGGESTIONS} systemHint={POS_HINT} />
            </Box>
          )}
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="POS sections" sx={{ width: { md: 190 }, flexShrink: 0 }}>
            <List
              orientation="horizontal"
              sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}
            >
              {NAV.map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton
                    selected={view === n.id}
                    onClick={() => setView(n.id)}
                    aria-current={view === n.id ? "page" : undefined}
                    data-testid={`pos-nav-${n.id}`}
                    sx={{ borderRadius: "md" }}
                  >
                    <ListItemDecorator>{n.icon}</ListItemDecorator>
                    <ListItemContent>{n.label}</ListItemContent>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            {loaded && view === "register" && (
              <RegisterView
                registers={registers}
                taxes={taxes}
                presets={presets}
                settings={settings}
                registerId={registerId}
                setRegisterId={setRegisterId}
                onChanged={changed}
              />
            )}
            {loaded && view === "sales" && (
              <SalesView registers={registers} settings={settings} registerId={registerId} version={version} onChanged={changed} />
            )}
            {loaded && view === "drawer" && <ShiftsView registers={registers} settings={settings} version={version} onChanged={changed} />}
            {loaded && view === "reports" && <ReportsView settings={settings} version={version} />}
            {loaded && view === "settings" && (
              <>
                <SettingsView registers={registers} taxes={taxes} presets={presets} settings={settings} onChanged={changed} />
                <AdvancedSettings registers={registers} settings={settings} onChanged={changed} />
              </>
            )}
          </Box>
        </Box>
      </Container>
    </>
  );
}
