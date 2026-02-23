import { sessions } from "../stores/sessions.svelte.js";
import { ui } from "../stores/ui.svelte.js";
import type { Session } from "../api/types.js";

const SAFE_ID = /^[a-zA-Z0-9_\-:.]+$/;

/**
 * Shell-escape a session ID by wrapping in single quotes.
 * Falls back to the raw id if it already matches safe characters.
 */
function shellEscape(id: string): string {
  if (SAFE_ID.test(id)) return id;
  return "'" + id.replace(/'/g, "'\\''") + "'";
}

/**
 * Return the CLI continue command for a session based on its agent type.
 * Pure function — no side effects.
 */
export function getSessionCommand(session: Session): string {
  const safeId = shellEscape(session.id);
  switch (session.agent) {
    case "claude":
      return `claude --continue ${safeId}`;
    case "codex":
      return `codex --continue ${safeId}`;
    default:
      return safeId;
  }
}

/**
 * Copy the continue command for a session to the clipboard
 * and show a toast notification.
 */
export function copySessionCommand(
  sessionId: string,
): void {
  const session = sessions.sessions.find(
    (s) => s.id === sessionId,
  );
  if (!session) return;

  const command = getSessionCommand(session);
  navigator.clipboard.writeText(command).then(
    () => {
      ui.showToast(`Copied: ${command}`);
    },
    () => {
      ui.showToast("Failed to copy to clipboard");
    },
  );
}
