import type { PinnedMessage } from "../api/types.js";
import * as api from "../api/client.js";

class PinsStore {
  /** All pins across all sessions (loaded for pinned tab). */
  pins: PinnedMessage[] = $state([]);
  loading: boolean = $state(false);

  /** Message IDs that are pinned in the currently viewed session. */
  sessionPinIds: Set<number> = $state(new Set());

  #currentSessionId: string | null = null;
  /** Maps messageId → sessionId for in-flight mutations. */
  #inflight: Map<number, string> = new Map();
  #loadVersion = 0;
  #loadAllVersion = 0;

  async loadAll() {
    this.loading = true;
    const version = ++this.#loadAllVersion;
    try {
      const res = await api.listPins();
      // Skip if a newer loadAll was issued or any mutation is
      // in-flight (this.pins is global, any mutation can conflict).
      if (this.#loadAllVersion === version && this.#inflight.size === 0) {
        this.pins = res.pins;
      }
    } finally {
      this.loading = false;
    }
  }

  async loadForSession(sessionId: string) {
    this.#currentSessionId = sessionId;
    const version = ++this.#loadVersion;
    this.sessionPinIds = new Set();
    try {
      const res = await api.listSessionPins(sessionId);
      // Only block on in-flight mutations for THIS session;
      // mutations for other sessions don't affect sessionPinIds.
      const hasInflightForSession = [...this.#inflight.values()].some(
        (sid) => sid === sessionId,
      );
      if (this.#loadVersion === version && !hasInflightForSession) {
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
    this.#inflight.set(messageId, sessionId);
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
      this.#inflight.set(messageId, sessionId);
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
