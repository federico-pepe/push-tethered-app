import { PushService } from "../bindings/github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui";
import type { ModuleInfo } from "../bindings/github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui/models";

// Switcher + install/uninstall, per plans/2026-08-17-module-host.md's phase-3
// scope plus the process-loader's own Install/Uninstall (phase 4, see
// plans/2026-08-17-process-loader.md) — list modules, show which is active,
// switch, and manage process-loaded ones. Still not in scope: per-module
// settings (seq's BPM, remap's overrides), still edited by hand-editing the
// config file the host logs on activation, same as from the CLI.

const listEl = document.getElementById("module-list") as HTMLUListElement;
const statusEl = document.getElementById("status") as HTMLParagraphElement;
const installBtn = document.getElementById("install-btn") as HTMLButtonElement;
const connectViewEl = document.getElementById("connect-view") as HTMLElement;
const portListEl = document.getElementById("port-list") as HTMLUListElement;
const retryBtn = document.getElementById("retry-btn") as HTMLButtonElement;

// busy covers any in-flight Activate/Install/Uninstall/Connect — all mutate
// shared state, so serialising them avoids a user's second click racing the
// first one's effects.
let busy = false;

async function refresh(): Promise<void> {
    let connected: boolean;
    try {
        connected = await PushService.IsConnected();
    } catch (err) {
        statusEl.textContent = `Could not reach the host: ${err}`;
        return;
    }

    if (!connected) {
        connectViewEl.hidden = false;
        listEl.hidden = true;
        installBtn.hidden = true;
        let lastError = "";
        try {
            lastError = await PushService.LastError();
        } catch {
            // Not fatal — just means the status line falls back below.
        }
        statusEl.textContent = lastError ? `Disconnected: ${lastError}` : "Not connected";
        await renderPorts();
        return;
    }
    connectViewEl.hidden = true;
    listEl.hidden = false;
    installBtn.hidden = false;

    let modules: ModuleInfo[];
    try {
        modules = await PushService.ListModules();
    } catch (err) {
        statusEl.textContent = `Could not reach the host: ${err}`;
        return;
    }
    render(modules);
}

async function renderPorts(): Promise<void> {
    retryBtn.disabled = busy;
    let ports: string[];
    try {
        ports = await PushService.ListMIDIPorts();
    } catch (err) {
        portListEl.innerHTML = `<li>Could not list MIDI ports: ${err}</li>`;
        return;
    }

    portListEl.innerHTML = "";
    if (ports.length === 0) {
        portListEl.innerHTML = "<li>No MIDI input ports found — is Push connected?</li>";
        return;
    }
    for (const name of ports) {
        const li = document.createElement("li");
        li.className = "port-row";

        const label = document.createElement("span");
        label.className = "port-name";
        label.textContent = name;

        const connectBtn = document.createElement("button");
        connectBtn.className = "port-connect";
        connectBtn.textContent = "Connect";
        connectBtn.disabled = busy;
        connectBtn.addEventListener("click", () => connect(name));

        li.append(label, connectBtn);
        portListEl.appendChild(li);
    }
}

async function connect(name: string): Promise<void> {
    if (busy) return;
    busy = true;
    statusEl.textContent = `Connecting to ${name}…`;
    try {
        await PushService.ConnectMIDIPort(name);
        busy = false;
        await refresh(); // switches to the module-list view
    } catch (err) {
        statusEl.textContent = `Could not connect: ${err}`;
        busy = false;
        await renderPorts(); // stay on the picker, re-enable its buttons
    }
}

function render(modules: ModuleInfo[]): void {
    const active = modules.find((m) => m.active);
    statusEl.textContent = active ? `Active: ${active.name}` : "No module active";
    installBtn.disabled = busy;

    listEl.innerHTML = "";
    for (const m of modules) {
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
        activateBtn.disabled = m.active || busy;
        activateBtn.addEventListener("click", () => activate(m.id));
        buttons.appendChild(activateBtn);

        // Only a process-loaded module can be uninstalled, and not while it's
        // the active one (Runtime.Uninstall refuses both, but there's no
        // reason to show a control here that we already know would fail).
        if (m.installed) {
            const uninstallBtn = document.createElement("button");
            uninstallBtn.className = "module-uninstall";
            uninstallBtn.textContent = "Uninstall";
            uninstallBtn.disabled = m.active || busy;
            uninstallBtn.title = m.active ? "Switch to another module first" : "";
            uninstallBtn.addEventListener("click", () => uninstall(m.id));
            buttons.appendChild(uninstallBtn);
        }

        li.append(info, buttons);
        listEl.appendChild(li);
    }
}

function badge(text: string): HTMLSpanElement {
    const el = document.createElement("span");
    el.className = "module-badge";
    el.textContent = text;
    return el;
}

async function activate(id: string): Promise<void> {
    if (busy) return;
    busy = true;
    statusEl.textContent = "Switching…";
    try {
        await PushService.ActivateModule(id);
    } catch (err) {
        statusEl.textContent = `Could not switch: ${err}`;
    } finally {
        busy = false;
        await refresh();
    }
}

async function install(): Promise<void> {
    if (busy) return;
    busy = true;
    installBtn.disabled = true;
    statusEl.textContent = "Choose a module folder…";
    try {
        const info = await PushService.InstallModulePrompt();
        // A zero-value ModuleInfo (empty id) means the user cancelled the
        // picker — PushService.InstallModulePrompt's documented way of telling
        // "cancelled" apart from "failed" without throwing for a non-error.
        if (info.id) {
            statusEl.textContent = `Installed ${info.name}`;
        }
    } catch (err) {
        statusEl.textContent = `Could not install: ${err}`;
    } finally {
        busy = false;
        await refresh();
    }
}

async function uninstall(id: string): Promise<void> {
    if (busy) return;
    busy = true;
    try {
        await PushService.UninstallModule(id);
    } catch (err) {
        statusEl.textContent = `Could not uninstall: ${err}`;
    } finally {
        busy = false;
        await refresh();
    }
}

installBtn.addEventListener("click", install);
retryBtn.addEventListener("click", () => connect(""));

// No push events from the host yet (phase 4's process loader is the natural
// point to add an "active module changed" event) — poll instead. Slow enough
// that it costs nothing, fast enough that a switch from the CLI in another
// terminal shows up promptly.
refresh();
setInterval(refresh, 2000);
