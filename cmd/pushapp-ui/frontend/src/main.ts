import { PushService } from "../bindings/github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui";
import type { ConnectRequest, ModuleInfo, Overview, SessionInfo } from "../bindings/github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui/models";
import type { Info as USBUnit } from "../bindings/github.com/federico-pepe/push-tethered-app/internal/display/models";
import type { PortRef, Unit as MIDIUnit } from "../bindings/github.com/federico-pepe/push-tethered-app/internal/midi/models";

// Pairing + switcher + install/uninstall. See plans/2026-08-19-multi-device.md
// for the design this implements: pair a USB unit with a MIDI cable manually
// (Identify blinks the screen or lights the pads so two identical units can
// be told apart), connect several at once, and manage each session's modules
// independently. Still not in scope: per-module settings (seq's BPM, remap's
// overrides), still edited by hand-editing the config file the host logs on
// activation, same as from the CLI.

const EXPECTED_API_VERSION = 2;

const statusEl = document.getElementById("status") as HTMLParagraphElement;
const installBtn = document.getElementById("install-btn") as HTMLButtonElement;

const pairingViewEl = document.getElementById("pairing-view") as HTMLElement;
const usbListEl = document.getElementById("usb-unit-list") as HTMLUListElement;
const midiListEl = document.getElementById("midi-unit-list") as HTMLUListElement;
const pairBtn = document.getElementById("pair-btn") as HTMLButtonElement;
const autoBtn = document.getElementById("auto-btn") as HTMLButtonElement;

const sessionListEl = document.getElementById("session-list") as HTMLUListElement;

// globalBusy covers connect/disconnect/pair/install — all mutate shared
// process-wide state, so serialising them avoids a user's second click racing
// the first one's effects. busySessions covers per-session activate/uninstall
// so one session's in-flight action does not freeze every other session's
// buttons — a real usability regression the single-session UI never had to
// worry about.
let globalBusy = false;
const busySessions = new Set<string>();

let selectedUSB: string | null = null; // USBUnit.id
let selectedMIDI: PortRef | null = null;
let identifying = new Set<string>(); // keys currently mid-flash, to disable their own button only

function apiChecked(): boolean {
    return (document.body.dataset.apiChecked ?? "") === "true";
}

async function checkAPIVersion(): Promise<boolean> {
    try {
        const version = await PushService.APIVersion();
        if (version !== EXPECTED_API_VERSION) {
            statusEl.textContent =
                `This window is out of date (backend API v${version}, UI expects v${EXPECTED_API_VERSION}) — reload the window.`;
            return false;
        }
    } catch (err) {
        statusEl.textContent = `Could not reach the host: ${err}`;
        return false;
    }
    document.body.dataset.apiChecked = "true";
    return true;
}

async function refresh(): Promise<void> {
    if (!apiChecked() && !(await checkAPIVersion())) {
        return;
    }

    let overview: Overview;
    try {
        overview = await PushService.Overview();
    } catch (err) {
        statusEl.textContent = `Could not reach the host: ${err}`;
        return;
    }

    const sessions = overview.sessions ?? [];
    const units = overview.units ?? [];
    const midiUnits = overview.midiUnits ?? [];

    const hasSessions = sessions.length > 0;
    const hasUnpairedUnits =
        units.some((u) => !sessions.some((s) => s.displaySel === u.id)) ||
        midiUnits.some((mu) => mu.ports.some((p) => !sessions.some((s) => s.midiIn.inNum === p.inNum)));

    pairingViewEl.hidden = !hasUnpairedUnits && hasSessions;
    sessionListEl.hidden = !hasSessions;
    installBtn.hidden = !hasSessions;

    statusEl.textContent = hasSessions
        ? `${sessions.length} unit${sessions.length === 1 ? "" : "s"} connected`
        : "Not connected";

    renderPairing(sessions, units, midiUnits, overview.unitErrors ?? {});
    await renderSessions(sessions);
}

function swatchFor(key: string): string {
    // A stable colour per unit key, so the same physical box gets the same
    // dot in the pairing view and (once connected) the same identify colour
    // — matching a screen to a row is then just "which dot is this".
    let hash = 0;
    for (let i = 0; i < key.length; i++) {
        hash = (hash * 31 + key.charCodeAt(i)) >>> 0;
    }
    const hue = hash % 360;
    return `hsl(${hue}, 70%, 55%)`;
}

function renderPairing(
    sessions: SessionInfo[],
    units: USBUnit[],
    midiUnits: MIDIUnit[],
    unitErrors: { [key: string]: string | undefined },
): void {
    const pairedDisplaySels = new Set(sessions.map((s) => s.displaySel).filter(Boolean));
    const pairedInNums = new Set(sessions.map((s) => s.midiIn.inNum));

    usbListEl.innerHTML = "";
    for (const unit of units) {
        if (pairedDisplaySels.has(unit.id)) continue;
        usbListEl.appendChild(renderUSBRow(unit, unitErrors[unit.id]));
    }
    if (usbListEl.children.length === 0) {
        usbListEl.innerHTML = "<li class=\"pairing-detail\">No unpaired screens found.</li>";
    }

    midiListEl.innerHTML = "";
    for (const unit of midiUnits) {
        for (const port of (unit.ports ?? [])) {
            if (!port.isLive) continue; // only the Live cable identifies a connectable unit
            if (pairedInNums.has(port.inNum)) continue;
            midiListEl.appendChild(renderMIDIRow(unit, port, unitErrors[unit.key]));
        }
    }
    if (midiListEl.children.length === 0) {
        midiListEl.innerHTML = "<li class=\"pairing-detail\">No unpaired MIDI ports found.</li>";
    }

    pairBtn.disabled = globalBusy || selectedUSB === null || selectedMIDI === null;
    autoBtn.disabled = globalBusy;
}

function renderUSBRow(unit: USBUnit, lastError: string | undefined): HTMLLIElement {
    const li = document.createElement("li");
    li.className = "pairing-row" + (selectedUSB === unit.id ? " is-selected" : "");
    li.addEventListener("click", () => {
        selectedUSB = selectedUSB === unit.id ? null : unit.id;
        void refresh();
    });

    const swatch = document.createElement("span");
    swatch.className = "pairing-swatch";
    swatch.style.background = swatchFor(unit.id);

    const info = document.createElement("span");
    info.className = "pairing-info";
    const name = document.createElement("span");
    name.className = "pairing-name";
    name.textContent = unit.model;
    const detail = document.createElement("span");
    detail.className = "pairing-detail";
    detail.textContent = lastError
        ? `Disconnected: ${lastError}`
        : unit.serial ? `serial ${unit.serial}` : unit.id;
    info.append(name, detail);

    const identifyBtn = document.createElement("button");
    identifyBtn.className = "pairing-identify";
    identifyBtn.textContent = identifying.has(unit.id) ? "Flashing…" : "Identify";
    identifyBtn.disabled = identifying.has(unit.id);
    identifyBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        void identifyUSB(unit.id);
    });

    li.append(swatch, info, identifyBtn);
    return li;
}

function renderMIDIRow(unit: MIDIUnit, port: PortRef, lastError: string | undefined): HTMLLIElement {
    const li = document.createElement("li");
    const isSelected = selectedMIDI?.inNum === port.inNum;
    const identifyKey = `out:${port.outNum}`;

    if (port.ambiguous) {
        li.className = "pairing-row is-disabled";
    } else {
        li.className = "pairing-row" + (isSelected ? " is-selected" : "");
        li.addEventListener("click", () => {
            selectedMIDI = isSelected ? null : port;
            void refresh();
        });
    }

    const swatch = document.createElement("span");
    swatch.className = "pairing-swatch";
    swatch.style.background = swatchFor(unit.key);

    const info = document.createElement("span");
    info.className = "pairing-info";
    const name = document.createElement("span");
    name.className = "pairing-name";
    name.textContent = `${unit.device} — ${unit.key}`;
    const detail = document.createElement("span");
    detail.className = "pairing-detail";
    detail.textContent = lastError
        ? `Disconnected: ${lastError}`
        : port.ambiguous
            ? "Matches another identical unit — identify by LED to tell them apart"
            : port.inName;
    info.append(name, detail);

    li.append(swatch, info);

    if (port.outNum >= 0) {
        const identifyBtn = document.createElement("button");
        identifyBtn.className = "pairing-identify";
        identifyBtn.textContent = identifying.has(identifyKey) ? "Flashing…" : "Identify";
        identifyBtn.disabled = identifying.has(identifyKey);
        identifyBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            void identifyMIDI(identifyKey, port.outNum);
        });
        li.appendChild(identifyBtn);
    }

    return li;
}

async function identifyUSB(id: string): Promise<void> {
    identifying.add(id);
    void refresh();
    try {
        await PushService.IdentifyUnit(id);
    } catch (err) {
        statusEl.textContent = `Could not identify: ${err}`;
    } finally {
        identifying.delete(id);
        await refresh();
    }
}

async function identifyMIDI(key: string, outNum: number): Promise<void> {
    identifying.add(key);
    void refresh();
    try {
        await PushService.IdentifyMIDIPort(outNum);
    } catch (err) {
        statusEl.textContent = `Could not identify: ${err}`;
    } finally {
        identifying.delete(key);
        await refresh();
    }
}

// emptyPortRef is the zero PortRef: an empty InName tells bootstrap.Open to
// fall back to auto-detecting the Live port (see ConnectRequest.unitKey and
// bootstrap.Options.MIDIIn's doc) rather than naming a specific cable.
const emptyPortRef: PortRef = {
    inName: "", outName: "", inNum: 0, outNum: -1,
    unit: "", cable: 0, role: "", device: undefined, isLive: false, ambiguous: false,
};

async function pairAndConnect(): Promise<void> {
    if (globalBusy || selectedUSB === null || selectedMIDI === null) return;
    globalBusy = true;
    statusEl.textContent = "Connecting…";
    try {
        const req: ConnectRequest = { displaySel: selectedUSB, midiIn: selectedMIDI, moduleId: "" };
        await PushService.Connect(req);
        selectedUSB = null;
        selectedMIDI = null;
    } catch (err) {
        statusEl.textContent = `Could not connect: ${err}`;
    } finally {
        globalBusy = false;
        await refresh();
    }
}

async function autoConnect(): Promise<void> {
    if (globalBusy) return;
    globalBusy = true;
    statusEl.textContent = "Connecting…";
    try {
        const req: ConnectRequest = { displaySel: "", midiIn: emptyPortRef, moduleId: "" };
        await PushService.Connect(req);
    } catch (err) {
        statusEl.textContent = `Could not auto-connect: ${err} — pick a screen and a MIDI port instead.`;
    } finally {
        globalBusy = false;
        await refresh();
    }
}

async function renderSessions(sessions: SessionInfo[]): Promise<void> {
    sessionListEl.innerHTML = "";
    for (const session of sessions) {
        sessionListEl.appendChild(await renderSessionCard(session));
    }
}

async function renderSessionCard(session: SessionInfo): Promise<HTMLLIElement> {
    const li = document.createElement("li");
    li.className = "session-card";
    li.dataset.key = session.key;

    const header = document.createElement("div");
    header.className = "session-header";

    const title = document.createElement("span");
    title.className = "session-title";
    const swatch = document.createElement("span");
    swatch.className = "pairing-swatch";
    swatch.style.background = swatchFor(session.unit);
    const unitLabel = document.createElement("span");
    unitLabel.className = "session-unit";
    unitLabel.textContent = session.midiIn.inName || session.unit;
    title.append(swatch, unitLabel);

    const buttons = document.createElement("span");
    buttons.className = "session-header-buttons";

    const busy = busySessions.has(session.key);

    if (session.displaySel) {
        const identifyBtn = document.createElement("button");
        identifyBtn.className = "pairing-identify";
        identifyBtn.textContent = "Identify";
        identifyBtn.disabled = busy;
        identifyBtn.addEventListener("click", () => identifySession(session));
        buttons.appendChild(identifyBtn);
    }

    const disconnectBtn = document.createElement("button");
    disconnectBtn.className = "session-disconnect";
    disconnectBtn.textContent = "Disconnect";
    disconnectBtn.disabled = busy;
    disconnectBtn.addEventListener("click", () => disconnectSession(session.key));
    buttons.appendChild(disconnectBtn);

    header.append(title, buttons);
    li.appendChild(header);

    let modules: ModuleInfo[];
    try {
        modules = (await PushService.ListModules(session.key)) ?? [];
    } catch (err) {
        const errEl = document.createElement("p");
        errEl.className = "session-error";
        errEl.textContent = `Could not list modules: ${err}`;
        li.appendChild(errEl);
        return li;
    }

    const list = document.createElement("ul");
    list.className = "module-list";
    for (const m of modules) {
        list.appendChild(renderModuleRow(session.key, m, busy));
    }
    li.appendChild(list);

    return li;
}

async function identifySession(session: SessionInfo): Promise<void> {
    if (busySessions.has(session.key)) return;
    busySessions.add(session.key);
    try {
        await PushService.IdentifyUnit(session.displaySel);
    } catch (err) {
        statusEl.textContent = `Could not identify: ${err}`;
    } finally {
        busySessions.delete(session.key);
        await refresh();
    }
}

async function disconnectSession(key: string): Promise<void> {
    if (busySessions.has(key)) return;
    busySessions.add(key);
    try {
        await PushService.Disconnect(key);
    } catch (err) {
        statusEl.textContent = `Could not disconnect: ${err}`;
    } finally {
        busySessions.delete(key);
        await refresh();
    }
}

function renderModuleRow(sessionKey: string, m: ModuleInfo, sessionBusy: boolean): HTMLLIElement {
    const li = document.createElement("li");
    li.className = "module-row" + (m.active ? " is-active" : "");

    const info = document.createElement("span");
    info.className = "module-info";

    const label = document.createElement("span");
    label.className = "module-name";
    label.textContent = m.version ? `${m.name} v${m.version}` : m.name;
    if (m.needsMidiOut) {
        label.appendChild(badge("MIDI out"));
    }
    if (m.installed) {
        label.appendChild(badge("installed"));
    }
    info.appendChild(label);

    if (m.description) {
        const desc = document.createElement("span");
        desc.className = "module-description";
        desc.textContent = m.description;
        info.appendChild(desc);
    }

    const buttons = document.createElement("span");
    buttons.className = "module-buttons";

    const activateBtn = document.createElement("button");
    activateBtn.className = "module-activate";
    activateBtn.textContent = m.active ? "Active" : "Activate";
    activateBtn.disabled = m.active || sessionBusy;
    activateBtn.addEventListener("click", () => activate(sessionKey, m.id));
    buttons.appendChild(activateBtn);

    if (m.installed) {
        const uninstallBtn = document.createElement("button");
        uninstallBtn.className = "module-uninstall";
        uninstallBtn.textContent = "Uninstall";
        uninstallBtn.disabled = m.active || sessionBusy;
        uninstallBtn.title = m.active ? "Switch to another module first" : "";
        uninstallBtn.addEventListener("click", () => uninstall(sessionKey, m.id));
        buttons.appendChild(uninstallBtn);
    }

    li.append(info, buttons);
    return li;
}

function badge(text: string): HTMLSpanElement {
    const el = document.createElement("span");
    el.className = "module-badge";
    el.textContent = text;
    return el;
}

async function activate(sessionKey: string, id: string): Promise<void> {
    if (busySessions.has(sessionKey)) return;
    busySessions.add(sessionKey);
    try {
        await PushService.ActivateModule(sessionKey, id);
    } catch (err) {
        statusEl.textContent = `Could not switch: ${err}`;
    } finally {
        busySessions.delete(sessionKey);
        await refresh();
    }
}

async function install(sessionKey: string): Promise<void> {
    if (globalBusy) return;
    globalBusy = true;
    installBtn.disabled = true;
    statusEl.textContent = "Choose a module folder…";
    try {
        const info = await PushService.InstallModulePrompt(sessionKey);
        // A zero-value ModuleInfo (empty id) means the user cancelled the
        // picker — InstallModulePrompt's documented way of telling
        // "cancelled" apart from "failed" without throwing for a non-error.
        if (info.id) {
            statusEl.textContent = `Installed ${info.name}`;
        }
    } catch (err) {
        statusEl.textContent = `Could not install: ${err}`;
    } finally {
        globalBusy = false;
        await refresh();
    }
}

async function uninstall(sessionKey: string, id: string): Promise<void> {
    if (busySessions.has(sessionKey)) return;
    busySessions.add(sessionKey);
    try {
        await PushService.UninstallModule(sessionKey, id);
    } catch (err) {
        statusEl.textContent = `Could not uninstall: ${err}`;
    } finally {
        busySessions.delete(sessionKey);
        await refresh();
    }
}

pairBtn.addEventListener("click", pairAndConnect);
autoBtn.addEventListener("click", autoConnect);
installBtn.addEventListener("click", () => {
    // Install through whichever session is first in the list — the
    // installed-modules directory is shared process-wide (see
    // PushService.InstallModulePrompt's doc), so any live session works.
    const firstKey = sessionListEl.querySelector<HTMLElement>(".session-card")?.dataset.key;
    void install(firstKey ?? "");
});

// No push events from the host yet (a natural addition once something needs
// them) — poll instead. Slow enough that it costs nothing, fast enough that a
// switch from the CLI in another terminal shows up promptly.
refresh();
setInterval(refresh, 2000);
