import type { PinnedMessage } from "../api/types.js";
import * as api from "../api/client.js";

class PinsStore {
  /** All pins across all sessions (loaded for pinned tab). */
  pins: PinnedMessage[] = $state([]);
  loading: boolean = $state(false);

  /** Message IDs that are pinned in the currently viewed session. */
  sessionPinIds: Set<number> = $state(new Set());

  #currentSessionId: string | null = null;
  #inflight: Set<number> = new Set();

  async loadAll() {
    this.loading = true;
    try {
      const res = await api.listPins();
      this.pins = res.pins;
    } finally {
      this.loading = false;
    }
  }

  async loadForSession(sessionId: string) {
    this.#currentSessionId = sessionId;
    this.sessionPinIds = new Set();
    try {
      const res = await api.listSessionPins(sessionId);
      // Guard against stale responses.
      if (this.#currentSessionId === sessionId) {
        this.sessionPinIds = new Set(
          res.pins.map((p) => p.message_id),
        );
      }
    } catch {
      // Silently ignore — pins are non-critical.
      // sessionPinIds was already cleared above.
    }
  }

  clearSession() {
    this.#currentSessionId = null;
    this.sessionPinIds = new Set();
  }

  isPinned(messageId: number): boolean {
    return this.sessionPinIds.has(messageId);
  }

  async unpin(sessionId: string, messageId: number) {
    if (this.#inflight.has(messageId)) return;
    this.#inflight.add(messageId);
    try {
      await api.unpinMessage(sessionId, messageId);
      const next = new Set(this.sessionPinIds);
      next.delete(messageId);
      this.sessionPinIds = next;
      this.pins = this.pins.filter(
        (p) =>
          !(
            p.session_id === sessionId &&
            p.message_id === messageId
          ),
      );
    } finally {
      this.#inflight.delete(messageId);
    }
  }

  async togglePin(
    sessionId: string,
    messageId: number,
    ordinal: number,
  ) {
    if (this.#inflight.has(messageId)) return;
    if (this.sessionPinIds.has(messageId)) {
      await this.unpin(sessionId, messageId);
    } else {
      this.#inflight.add(messageId);
      try {
        const result = await api.pinMessage(sessionId, messageId);
        const next = new Set(this.sessionPinIds);
        next.add(messageId);
        this.sessionPinIds = next;
        this.pins = [
          {
            id: result.id,
            session_id: sessionId,
            message_id: messageId,
            ordinal,
            created_at: new Date().toISOString(),
          },
          ...this.pins,
        ];
      } finally {
        this.#inflight.delete(messageId);
      }
    }
  }
}

export const pins = new PinsStore();
