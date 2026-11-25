// Receipt printer and cash drawer. Browsers cannot reach a USB printer without
// WebUSB (Chrome and Edge on desktop, over HTTPS or localhost). Everything is
// feature-detected: when WebUSB is missing, the caller shows a plain message and
// falls back to the browser print dialog.

const ESC = 0x1b;
const GS = 0x1d;

/** ESC/POS "open cash drawer" on pin 2: ESC p m t1 t2. */
export const DRAWER_KICK = new Uint8Array([ESC, 0x70, 0x00, 0x19, 0xfa]);

const enc = new TextEncoder();

/** Builds the bytes for a text receipt: init, text, feed, partial cut, optional drawer kick. */
export function receiptBytes(text: string, opts: { kickDrawer?: boolean; cut?: boolean } = {}): Uint8Array {
  const parts: Uint8Array[] = [new Uint8Array([ESC, 0x40])]; // initialise
  // Printers speak 7-bit ASCII by default: replace characters they would garble.
  const safe = text.replace(/[^\x09\x0a\x20-\x7e]/g, (c) => (c === "₹" ? "Rs." : c === "€" ? "EUR" : "?"));
  parts.push(enc.encode(safe.endsWith("\n") ? safe : safe + "\n"));
  parts.push(new Uint8Array([ESC, 0x64, 0x04])); // feed 4 lines
  if (opts.cut !== false) parts.push(new Uint8Array([GS, 0x56, 0x42, 0x00])); // feed and partial cut
  if (opts.kickDrawer) parts.push(DRAWER_KICK);
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let o = 0;
  for (const p of parts) {
    out.set(p, o);
    o += p.length;
  }
  return out;
}

export interface UsbSupport {
  webusb: boolean;
  reason: string;
}

/** Whether this browser can talk to a USB printer, with a human reason when it cannot. */
export function usbSupport(): UsbSupport {
  if (typeof navigator === "undefined" || !("usb" in navigator)) {
    return { webusb: false, reason: "This browser cannot use USB printers (WebUSB). Use Chrome or Edge on a computer, or print through the browser's print dialog." };
  }
  if (typeof window !== "undefined" && !window.isSecureContext) {
    return { webusb: false, reason: "USB printing needs a secure page (https or localhost)." };
  }
  return { webusb: true, reason: "" };
}

// Minimal WebUSB typings: lib.dom does not ship them.
interface UsbEndpoint {
  endpointNumber: number;
  direction: "in" | "out";
  type: string;
}
interface UsbDevice {
  productName?: string;
  opened: boolean;
  configuration: { interfaces: { interfaceNumber: number; alternates: { interfaceClass: number; endpoints: UsbEndpoint[] }[] }[] } | null;
  open(): Promise<void>;
  selectConfiguration(n: number): Promise<void>;
  claimInterface(n: number): Promise<void>;
  transferOut(ep: number, data: BufferSource): Promise<{ status: string }>;
}
interface UsbApi {
  requestDevice(o: { filters: { classCode?: number }[] }): Promise<UsbDevice>;
  getDevices(): Promise<UsbDevice[]>;
}

let device: UsbDevice | null = null;
let endpoint = 0;
let iface = 0;

async function prepare(d: UsbDevice) {
  if (!d.opened) await d.open();
  if (!d.configuration) await d.selectConfiguration(1);
  for (const i of d.configuration!.interfaces) {
    for (const a of i.alternates) {
      const out = a.endpoints.find((e) => e.direction === "out" && e.type === "bulk");
      if (a.interfaceClass === 7 && out) {
        iface = i.interfaceNumber;
        endpoint = out.endpointNumber;
        await d.claimInterface(iface);
        device = d;
        return;
      }
    }
  }
  throw new Error("This USB device has no printer interface.");
}

/** Asks the user to pick a USB printer (must run from a click). */
export async function connectPrinter(): Promise<string> {
  const s = usbSupport();
  if (!s.webusb) throw new Error(s.reason);
  const usb = (navigator as unknown as { usb: UsbApi }).usb;
  const d = await usb.requestDevice({ filters: [{ classCode: 7 }] });
  await prepare(d);
  return d.productName || "USB printer";
}

/** Reconnects to a printer the user already allowed, without a prompt. */
export async function reconnectPrinter(): Promise<string | null> {
  if (!usbSupport().webusb) return null;
  try {
    const usb = (navigator as unknown as { usb: UsbApi }).usb;
    const d = (await usb.getDevices()).find((x) => x.configuration?.interfaces.some((i) => i.alternates.some((a) => a.interfaceClass === 7)) || true);
    if (!d) return null;
    await prepare(d);
    return d.productName || "USB printer";
  } catch {
    return null;
  }
}

export const printerConnected = () => device !== null;

/** Sends bytes to the connected printer. */
export async function sendToPrinter(bytes: Uint8Array): Promise<void> {
  if (!device) throw new Error("No USB printer is connected. Connect one first.");
  const r = await device.transferOut(endpoint, bytes as unknown as BufferSource);
  if (r.status !== "ok") throw new Error(`The printer did not accept the data (${r.status}).`);
}

/** Opens the cash drawer through the printer's drawer port. */
export const kickDrawer = () => sendToPrinter(DRAWER_KICK);
