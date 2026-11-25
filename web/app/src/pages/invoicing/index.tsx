import { useCallback, useEffect, useMemo, useState } from "react";
import { BizAssistant } from "../../components/BizAssistant";
import { onAiChanged, withAccess } from "../../lib/ai/biz";
import { usePermissions } from "../../lib/permissions";
import { createSalesTools, SALES_HINT, SALES_SUGGESTIONS } from "./ai/tools";
import { Box, CircularProgress, Container, List, ListItem, ListItemButton, ListItemContent, ListItemDecorator, Typography } from "@mui/joy";
import ReceiptLongIcon from "@mui/icons-material/ReceiptLong";
import DescriptionOutlined from "@mui/icons-material/DescriptionOutlined";
import RequestQuoteOutlined from "@mui/icons-material/RequestQuoteOutlined";
import AutorenewIcon from "@mui/icons-material/Autorenew";
import PeopleOutline from "@mui/icons-material/PeopleOutline";
import BarChartIcon from "@mui/icons-material/BarChart";
import SellOutlined from "@mui/icons-material/SellOutlined";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import ShoppingCartOutlined from "@mui/icons-material/ShoppingCartOutlined";
import LocalShippingOutlined from "@mui/icons-material/LocalShippingOutlined";
import ReceiptOutlined from "@mui/icons-material/ReceiptOutlined";
import PercentOutlined from "@mui/icons-material/PercentOutlined";
import HourglassBottomOutlined from "@mui/icons-material/HourglassBottomOutlined";
import CardMembershipOutlined from "@mui/icons-material/CardMembershipOutlined";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import { getCapabilities, getSettings, listItems, listTaxRates, runRecurring } from "./api";
import { DocsView } from "./DocsView";
import { ClientsView, ItemsView, RecurringView, ReportsView, SettingsView } from "./OtherViews";
import { DeliveriesView, OrdersView } from "./OrdersView";
import { AgingView, PricingView, SubscriptionsView } from "./SalesViews";
import type { Item, Settings, TaxRate } from "./types";
import { ACCENT } from "./logic";
import { deepParam } from "../../lib/deepLink";

type View = "estimates" | "orders" | "deliveries" | "invoices" | "credits" | "recurring" | "subscriptions" | "clients" | "aging" | "pricing" | "reports" | "items" | "settings";

const NAV: { id: View; label: string; icon: JSX.Element }[] = [
  { id: "estimates", label: "Estimates", icon: <RequestQuoteOutlined /> },
  { id: "orders", label: "Sales orders", icon: <ShoppingCartOutlined /> },
  { id: "deliveries", label: "Delivery notes", icon: <LocalShippingOutlined /> },
  { id: "invoices", label: "Invoices", icon: <DescriptionOutlined /> },
  { id: "credits", label: "Credit notes", icon: <ReceiptOutlined /> },
  { id: "recurring", label: "Recurring", icon: <AutorenewIcon /> },
  { id: "subscriptions", label: "Subscriptions", icon: <CardMembershipOutlined /> },
  { id: "clients", label: "Clients", icon: <PeopleOutline /> },
  { id: "aging", label: "Who owes", icon: <HourglassBottomOutlined /> },
  { id: "pricing", label: "Pricing", icon: <PercentOutlined /> },
  { id: "reports", label: "Reports", icon: <BarChartIcon /> },
  { id: "items", label: "Price list", icon: <SellOutlined /> },
  { id: "settings", label: "Settings", icon: <SettingsOutlined /> },
];

export default function InvoicingApp({ user }: { user: User }) {
  const [view, setView] = useState<View>("invoices");
  const [settings, setSettings] = useState<Settings | null>(null);
  const [items, setItems] = useState<Item[]>([]);
  const [taxes, setTaxes] = useState<TaxRate[]>([]);
  const [emailReady, setEmailReady] = useState(false);
  const [version, setVersion] = useState(0);
  const [preset, setPreset] = useState("");
  const [openDoc, setOpenDoc] = useState(() => deepParam("doc"));
  const [openOrder, setOpenOrder] = useState("");
  const clearDoc = useCallback(() => setOpenDoc(""), []);
  const clearOrder = useCallback(() => setOpenOrder(""), []);
  const gotoDoc = (id: string, v: View) => {
    setOpenDoc(id);
    setView(v);
  };
  const [error, setError] = useState("");

  const reload = useCallback(async () => {
    try {
      const [s, i, t, c] = await Promise.all([
        getSettings(),
        listItems().catch(() => [] as Item[]),
        listTaxRates(),
        getCapabilities().catch(() => ({ email: false, stock: false })),
      ]);
      setSettings(s);
      setItems(i);
      setTaxes(t);
      setEmailReady(c.email);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    reload();
  }, [reload, version]);

  // Create any recurring invoices that came due, once per visit.
  useEffect(() => {
    runRecurring()
      .then((made) => made.length && setVersion((v) => v + 1))
      .catch(() => undefined);
  }, []);

  const changed = useCallback(() => setVersion((v) => v + 1), []);
  const { can: canDo } = usePermissions();
  const aiTools = useMemo(() => withAccess(createSalesTools(), canDo("invoicing:document:write")), [canDo]);
  useEffect(() => onAiChanged("invoicing", changed), [changed]);

  return (
    <>
      <Header user={user} />
      <Container maxWidth="xl" sx={{ py: { xs: 2, sm: 3 }, px: { xs: 1.5, sm: 3 } }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2 }}>
          <ReceiptLongIcon sx={{ fontSize: 28, color: ACCENT }} />
          <Typography level="h2" sx={{ fontSize: { xs: "xl", sm: "xl3" } }}>
            Invoicing
          </Typography>
          <Box sx={{ ml: "auto" }}>
            <BizAssistant app="invoicing" title="Invoicing and sales" tools={aiTools} suggestions={SALES_SUGGESTIONS} systemHint={SALES_HINT} />
          </Box>
        </Box>
        <Box sx={{ display: "flex", flexDirection: { xs: "column", md: "row" }, gap: { xs: 1, md: 3 } }}>
          <Box component="nav" aria-label="Invoicing sections" sx={{ width: { md: 190 }, flexShrink: 0 }}>
            <List orientation="horizontal" sx={{ flexWrap: "wrap", "@media (min-width: 900px)": { flexDirection: "column", flexWrap: "nowrap" }, gap: 0.5 }}>
              {NAV.map((n) => (
                <ListItem key={n.id} sx={{ width: { md: "100%" } }}>
                  <ListItemButton
                    selected={view === n.id}
                    onClick={() => {
                      setPreset("");
                      setView(n.id);
                    }}
                    aria-current={view === n.id ? "page" : undefined}
                    data-testid={`invc-nav-${n.id}`}
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
            {error && (
              <Typography color="danger" role="alert">
                {error}
              </Typography>
            )}
            {!settings && !error && <CircularProgress />}
            {settings && (view === "invoices" || view === "estimates" || view === "credits") && (
              <DocsView
                key={view}
                kind={view === "invoices" ? "invoice" : view === "credits" ? "credit_note" : "estimate"}
                openId={openDoc}
                clearOpen={clearDoc}
                onOpenOrder={(id) => {
                  setOpenOrder(id);
                  setView("orders");
                }}
                settings={settings}
                items={items}
                taxes={taxes}
                emailReady={emailReady}
                version={version}
                onChanged={changed}
                presetQuery={preset}
              />
            )}
            {settings && view === "orders" && (
              <OrdersView settings={settings} version={version} onChanged={changed} openId={openOrder} clearOpen={clearOrder} onOpenInvoice={(id) => gotoDoc(id, "invoices")} />
            )}
            {settings && view === "deliveries" && <DeliveriesView settings={settings} version={version} />}
            {settings && view === "subscriptions" && <SubscriptionsView settings={settings} version={version} onChanged={changed} />}
            {settings && view === "aging" && <AgingView settings={settings} version={version} />}
            {settings && view === "pricing" && <PricingView items={items} />}
            {settings && view === "recurring" && <RecurringView settings={settings} version={version} onChanged={changed} />}
            {settings && view === "clients" && (
              <ClientsView
                settings={settings}
                version={version}
                onOpenClient={(c) => {
                  setPreset(c.client_name);
                  setView("invoices");
                }}
              />
            )}
            {settings && view === "reports" && <ReportsView version={version} />}
            {settings && view === "items" && <ItemsView items={items} settings={settings} onChanged={changed} />}
            {settings && view === "settings" && <SettingsView settings={settings} onSaved={changed} />}
          </Box>
        </Box>
      </Container>
    </>
  );
}
