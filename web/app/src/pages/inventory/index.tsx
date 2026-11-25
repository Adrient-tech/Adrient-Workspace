import { useCallback, useEffect, useMemo, useState } from "react";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged, withAccess } from "../../lib/ai/biz";
import { usePermissions } from "../../lib/permissions";
import { createInventoryTools, INV_HINT, INV_SUGGESTIONS } from "./ai/tools";
import {
  Box,
  Chip,
  Container,
  List,
  ListItem,
  ListItemButton,
  ListItemContent,
  ListItemDecorator,
  Typography,
} from "@mui/joy";
import Inventory2Icon from "@mui/icons-material/Inventory2";
import CategoryOutlined from "@mui/icons-material/CategoryOutlined";
import ShoppingCartOutlined from "@mui/icons-material/ShoppingCartOutlined";
import HistoryIcon from "@mui/icons-material/History";
import BarChartIcon from "@mui/icons-material/BarChart";
import LocalShippingOutlined from "@mui/icons-material/LocalShippingOutlined";
import WarehouseOutlined from "@mui/icons-material/WarehouseOutlined";
import SwapHorizIcon from "@mui/icons-material/SwapHoriz";
import PaidOutlined from "@mui/icons-material/PaidOutlined";
import FactCheckOutlined from "@mui/icons-material/FactCheckOutlined";
import InsightsIcon from "@mui/icons-material/Insights";
import BuildOutlined from "@mui/icons-material/BuildOutlined";
import ReceiptLongOutlined from "@mui/icons-material/ReceiptLongOutlined";
import LabelOutlined from "@mui/icons-material/LabelOutlined";
import QrCodeScannerIcon from "@mui/icons-material/QrCodeScanner";
import EventNoteOutlined from "@mui/icons-material/EventNoteOutlined";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import { listLocations, listProducts, listSuppliers } from "./api";
import { HistoryView, ReorderView, ReportsView, SuppliersView } from "./OtherViews";
import { ProductsView } from "./ProductsView";
import { WarehousesView } from "./Warehouses";
import { TransfersView } from "./Transfers";
import { ValuationView } from "./Valuation";
import { LotsView } from "./Lots";
import { PurchasingView } from "./Purchasing";
import { CountsView, PlanningView } from "./Counts";
import { AssemblyView } from "./Assembly";
import { LabelsView } from "./Labels";
import { ScanPage } from "./ScanPage";
import type { InvLocation, InvSupplier } from "./types";

type View =
  | "products" | "warehouses" | "transfers" | "lots" | "counts" | "planning" | "assembly"
  | "purchasing" | "reorder" | "suppliers"
  | "valuation" | "history" | "reports"
  | "labels" | "scan";

const NAV: { section: string; items: { id: View; label: string; icon: JSX.Element }[] }[] = [
  {
    section: "Stock",
    items: [
      { id: "products", label: "Products", icon: <CategoryOutlined /> },
      { id: "warehouses", label: "Warehouses", icon: <WarehouseOutlined /> },
      { id: "transfers", label: "Transfers", icon: <SwapHorizIcon /> },
      { id: "lots", label: "Lots and serials", icon: <EventNoteOutlined /> },
      { id: "counts", label: "Stock counts", icon: <FactCheckOutlined /> },
      { id: "assembly", label: "Kits and assembly", icon: <BuildOutlined /> },
    ],
  },
  {
    section: "Buying",
    items: [
      { id: "purchasing", label: "Purchasing", icon: <ReceiptLongOutlined /> },
      { id: "reorder", label: "Reorder", icon: <ShoppingCartOutlined /> },
      { id: "suppliers", label: "Suppliers", icon: <LocalShippingOutlined /> },
    ],
  },
  {
    section: "Insight",
    items: [
      { id: "valuation", label: "Valuation", icon: <PaidOutlined /> },
      { id: "planning", label: "Planning", icon: <InsightsIcon /> },
      { id: "history", label: "History", icon: <HistoryIcon /> },
      { id: "reports", label: "Reports", icon: <BarChartIcon /> },
    ],
  },
  {
    section: "Tools",
    items: [
      { id: "labels", label: "Barcode labels", icon: <LabelOutlined /> },
      { id: "scan", label: "Scan (phone)", icon: <QrCodeScannerIcon /> },
    ],
  },
];

export default function InventoryApp({ user }: { user: User }) {
  const initial = (new URLSearchParams(window.location.search).get("view") as View | null) ?? "products";
  const [view, setView] = useState<View>(initial);
  const [buyTab, setBuyTab] = useState<"orders" | "reorder">("orders");
  const [locations, setLocations] = useState<InvLocation[]>([]);
  const [suppliers, setSuppliers] = useState<InvSupplier[]>([]);
  const [lowCount, setLowCount] = useState(0);
  const [version, setVersion] = useState(0);

  const reload = useCallback(async () => {
    const [l, s, low] = await Promise.all([
      listLocations().catch(() => []),
      listSuppliers().catch(() => []),
      listProducts({ low_stock_only: true, flat: true }).catch(() => []),
    ]);
    setLocations(l);
    setSuppliers(s);
    setLowCount(low.length);
  }, []);

  useEffect(() => {
    reload();
  }, [reload, version]);

  const changed = useCallback(() => setVersion((v) => v + 1), []);
  const { can: canDo } = usePermissions();
  const aiTools = useMemo(() => withAccess(createInventoryTools(), canDo("inventory:product:write")), [canDo]);
  useEffect(() => onAiChanged("inventory", changed), [changed]);

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <Inventory2Icon sx={{ fontSize: 28, color: "#0E7C66" }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>
            Inventory
          </Typography>
          <Box sx={{ ml: "auto" }}>
            <BizAssistant app="inventory" title="Inventory" tools={aiTools} suggestions={INV_SUGGESTIONS} systemHint={INV_HINT} />
          </Box>
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Inventory sections" sx={{ width: { md: 200 }, flexShrink: 0 }}>
            <List
              orientation="horizontal"
              sx={{
                flexWrap: "wrap",
                "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" },
                gap: 0.5,
              }}
            >
              {NAV.map((g) => (
                <Box key={g.section} sx={{ display: "contents" }}>
                  <ListItem sx={{ width: { md: "100%" }, display: { xs: "none", md: "flex" } }}>
                    <Typography level="body-xs" sx={{ textTransform: "uppercase", letterSpacing: 0.6, mt: 1, px: 1 }}>{g.section}</Typography>
                  </ListItem>
                  {g.items.map((n) => (
                    <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                      <ListItemButton
                        selected={view === n.id}
                        onClick={() => { if (n.id === "purchasing") setBuyTab("orders"); setView(n.id); }}
                        aria-current={view === n.id ? "page" : undefined}
                        data-testid={`inv-nav-${n.id}`}
                        sx={{ borderRadius: "md" }}
                      >
                        <ListItemDecorator>{n.icon}</ListItemDecorator>
                        <ListItemContent>{n.label}</ListItemContent>
                        {n.id === "reorder" && lowCount > 0 && (
                          <Chip size="sm" color="warning" variant="soft">
                            {lowCount}
                          </Chip>
                        )}
                      </ListItemButton>
                    </ListItem>
                  ))}
                </Box>
              ))}
            </List>
          </Box>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            {view === "products" && (
              <ProductsView locations={locations} suppliers={suppliers} version={version} onChanged={changed} />
            )}
            {view === "warehouses" && <WarehousesView version={version} onChanged={changed} />}
            {view === "transfers" && <TransfersView version={version} onChanged={changed} />}
            {view === "lots" && <LotsView version={version} onChanged={changed} />}
            {view === "counts" && <CountsView version={version} onChanged={changed} />}
            {view === "assembly" && <AssemblyView version={version} onChanged={changed} />}
            {view === "purchasing" && <PurchasingView key={buyTab} initialTab={buyTab} suppliers={suppliers} version={version} onChanged={changed} />}
            {view === "reorder" && <ReorderView version={version} onPlan={() => { setBuyTab("reorder"); setView("purchasing"); }} />}
            {view === "suppliers" && <SuppliersView suppliers={suppliers} onChanged={changed} />}
            {view === "valuation" && <ValuationView version={version} />}
            {view === "planning" && <PlanningView version={version} onChanged={changed} />}
            {view === "history" && <HistoryView version={version} />}
            {view === "reports" && <ReportsView version={version} />}
            {view === "labels" && <LabelsView />}
            {view === "scan" && <ScanPage version={version} onChanged={changed} />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
