import { ui } from "../stores/ui.svelte.js";
import { sessions } from "../stores/sessions.svelte.js";
import { starred } from "../stores/starred.svelte.js";
import { sync } from "../stores/sync.svelte.js";
import { getExportUrl } from "../api/client.js";

function isInputFocused(): boolean {
  const el = document.activeElement;
  if (!el) return false;
  const tag = el.tagName;
  return (
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    tag === "SELECT" ||
    (el as HTMLElement).isContentEditable
  );
}

interface ShortcutOptions {
  navigateMessage: (delta: number) => void;
}

function handleEscape(): void {
  if (ui.activeModal !== null) {
    ui.activeModal = null;
    return;
  }
  if (sessions.activeSessionId && !isInputFocused()) {
    sessions.deselectSession();
  }
}

/**
 * Register global keyboard shortcuts.
 * Returns a cleanup function to remove the listener.
 */
export function registerShortcuts(
  opts: ShortcutOptions,
): () => void {
  function handler(e: KeyboardEvent) {
    const meta = e.metaKey || e.ctrlKey;

    // Cmd+K — always works
    if (meta && e.key === "k") {
      e.preventDefault();
      ui.activeModal =
        ui.activeModal === "commandPalette"
          ? null
          : "commandPalette";
      return;
    }

    // Esc — always works
    if (e.key === "Escape") {
      handleEscape();
      return;
    }

    // All remaining shortcuts are plain single-key — skip if any modifier is held.
    // (Shift is allowed because "?" requires Shift on most layouts.)
    if (e.metaKey || e.ctrlKey || e.altKey) return;

    // All other shortcuts: skip when modal open or input focused
    if (ui.activeModal !== null || isInputFocused()) return;

    const keyActions: Record<string, () => void> = {
      j: () => opts.navigateMessage(1),
      ArrowDown: () => opts.navigateMessage(1),
      k: () => opts.navigateMessage(-1),
      ArrowUp: () => opts.navigateMessage(-1),
      "]": () => {
        const filter = starred.filterOnly
          ? (s: { id: string }) => starred.isStarred(s.id)
          : undefined;
        sessions.navigateSession(1, filter);
      },
      "[": () => {
        const filter = starred.filterOnly
          ? (s: { id: string }) => starred.isStarred(s.id)
          : undefined;
        sessions.navigateSession(-1, filter);
      },
      o: () => ui.toggleSort(),
      t: () => ui.toggleThinking(),
      l: () => ui.cycleLayout(),
      r: () => sync.triggerSync(() => sessions.load()),
      e: () => {
        if (sessions.activeSessionId) {
          window.open(
            getExportUrl(sessions.activeSessionId),
            "_blank",
          );
        }
      },
      p: () => {
        if (sessions.activeSessionId) {
          ui.activeModal = "publish";
        }
      },
      s: () => {
        if (sessions.activeSessionId) {
          starred.toggle(sessions.activeSessionId);
        }
      },
      "?": () => {
        ui.activeModal = "shortcuts";
      },
    };

    const action = keyActions[e.key];
    if (action) {
      e.preventDefault();
      action();
    }
  }

  document.addEventListener("keydown", handler);
  return () => document.removeEventListener("keydown", handler);
}
