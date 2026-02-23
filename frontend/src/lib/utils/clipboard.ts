import { sessions } from "../stores/sessions.svelte.js";
import { ui } from "../stores/ui.svelte.js";
import type { Session } from "../api/types.js";

const SAFE_ID = /^[a-zA-Z0-9_\-:.]+$/;

/**
 * Return the CLI continue command for a session based on its agent type.
 * Returns null if the session ID contains unsafe characters.
 */
export function getSessionCommand(session: Session): string | null {
  if (!SAFE_ID.test(session.id)) return null;
  switch (session.agent) {
    case "claude":
      return `claude --continue ${session.id}`;
    case "codex":
      return `codex --continue ${session.id}`;
    default:
      return session.id;
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
  if (command === null) {
    ui.showToast("Cannot copy: session ID contains unsafe characters");
    return;
  }
  navigator.clipboard.writeText(command).then(
    () => {
      ui.showToast(`Copied: ${command}`);
    },
    () => {
      ui.showToast("Failed to copy to clipboard");
    },
  );
}
