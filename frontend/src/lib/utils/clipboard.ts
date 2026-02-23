import { sessions } from "../stores/sessions.svelte.js";
import { ui } from "../stores/ui.svelte.js";
import type { Session } from "../api/types.js";

/**
 * Return the CLI continue command for a session based on its agent type.
 * Pure function — no side effects.
 */
export function getSessionCommand(session: Session): string {
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
  navigator.clipboard.writeText(command).then(() => {
    ui.showToast(`Copied: ${command}`);
  });
}
