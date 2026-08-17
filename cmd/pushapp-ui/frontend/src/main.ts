import { PushService } from "../bindings/github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui";
import type { ModuleInfo } from "../bindings/github.com/federico-pepe/push-tethered-app/cmd/pushapp-ui/models";

// Minimal switcher, per plans/2026-08-17-module-host.md's phase-3 scope:
// list modules, show which is active, let the user switch. Nothing else —
// per-module settings (seq's BPM, remap's overrides) are still edited by
// hand-editing the config file the host logs on activation, same as from the
// CLI, until a later phase adds a settings view.

const listEl = document.getElementById("module-list") as HTMLUListElement;
const statusEl = document.getElementById("status") as HTMLParagraphElement;

// switching is true while an ActivateModule call is in flight, so a second
// click can't race the first one — Activate is not free (it calls Close on
// the outgoing module and Init on the incoming one) and the list should
// reflect one switch at a time.
let switching = false;

async function refresh(): Promise<void> {
    let modules: ModuleInfo[];
    try {
        modules = await PushService.ListModules();
    } catch (err) {
        statusEl.textContent = `Could not reach the host: ${err}`;
        return;
    }
    render(modules);
}

function render(modules: ModuleInfo[]): void {
    const active = modules.find((m) => m.active);
    statusEl.textContent = active ? `Active: ${active.name}` : "No module active";

    listEl.innerHTML = "";
    for (const m of modules) {
        const li = document.createElement("li");
        li.className = "module-row" + (m.active ? " is-active" : "");

        const label = document.createElement("span");
        label.className = "module-name";
        label.textContent = m.name;
        if (m.needsMidiOut) {
            const badge = document.createElement("span");
            badge.className = "module-badge";
            badge.textContent = "MIDI out";
            label.appendChild(badge);
        }

        const button = document.createElement("button");
        button.className = "module-activate";
        button.textContent = m.active ? "Active" : "Activate";
        button.disabled = m.active || switching;
        button.addEventListener("click", () => activate(m.id));

        li.append(label, button);
        listEl.appendChild(li);
    }
}

async function activate(id: string): Promise<void> {
    if (switching) return;
    switching = true;
    statusEl.textContent = "Switching…";
    try {
        await PushService.ActivateModule(id);
    } catch (err) {
        statusEl.textContent = `Could not switch: ${err}`;
    } finally {
        switching = false;
        await refresh();
    }
}

// No push events from the host yet (phase 4's process loader is the natural
// point to add an "active module changed" event) — poll instead. Slow enough
// that it costs nothing, fast enough that a switch from the CLI in another
// terminal shows up promptly.
refresh();
setInterval(refresh, 2000);
