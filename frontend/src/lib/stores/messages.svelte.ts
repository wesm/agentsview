import * as api from "../api/client.js";
import type { Message } from "../api/types.js";

const MESSAGE_PAGE_SIZE = 1000;
const FULL_SESSION_MESSAGE_THRESHOLD = 20_000;

class MessagesStore {
  messages: Message[] = $state([]);
  loading: boolean = $state(false);
  sessionId: string | null = $state(null);
  messageCount: number = $state(0);
  hasOlder: boolean = $state(false);
  loadingOlder: boolean = $state(false);
  private reloadPromise: Promise<void> | null = null;
  private reloadSessionId: string | null = null;
  private pendingReload: boolean = false;

  async loadSession(id: string) {
    if (
      this.sessionId === id &&
      (this.messages.length > 0 || this.loading)
    ) {
      return;
    }
    this.sessionId = id;
    this.loading = true;
    this.messages = [];
    this.messageCount = 0;
    this.hasOlder = false;
    this.loadingOlder = false;
    this.reloadPromise = null;
    this.reloadSessionId = null;
    this.pendingReload = false;

    try {
      let countHint: number | null = null;
      try {
        const sess = await api.getSession(id);
        if (this.sessionId !== id) return;
        countHint = sess.message_count ?? 0;
      } catch {
        // Best-effort: if this fails we still attempt loading
        // messages directly.
      }

      if (
        countHint !== null &&
        countHint > FULL_SESSION_MESSAGE_THRESHOLD
      ) {
        await this.loadProgressively(id);
      } else {
        await this.loadAllMessages(id, countHint ?? undefined);
      }
    } catch {
      // Non-fatal. Active session may have changed or the
      // source file may be mid-write during sync.
    } finally {
      if (this.sessionId === id) {
        this.loading = false;
      }
    }
  }

  reload(): Promise<void> {
    if (!this.sessionId) return Promise.resolve();

    // Use the session ID of the current reload to ensure we don't return
    // a promise for a previous session.
    if (this.reloadPromise && this.reloadSessionId === this.sessionId) {
      this.pendingReload = true;
      return this.reloadPromise;
    }

    const id = this.sessionId;
    this.reloadSessionId = id;

    const promise = this.reloadNow(id).finally(async () => {
      if (this.reloadPromise === promise) {
        this.reloadPromise = null;
        this.reloadSessionId = null;
      }
      if (this.pendingReload && this.sessionId === id) {
        this.pendingReload = false;
        await this.reload();
      }
    });
    this.reloadPromise = promise;
    return promise;
  }

  clear() {
    this.messages = [];
    this.sessionId = null;
    this.loading = false;
    this.messageCount = 0;
    this.hasOlder = false;
    this.loadingOlder = false;
    this.reloadPromise = null;
    this.reloadSessionId = null;
    this.pendingReload = false;
  }

  private async loadAllMessages(
    id: string,
    messageCountHint?: number,
  ) {
    let from = 0;
    let loaded: Message[] = [];

    for (;;) {
      if (this.sessionId !== id) return;
      const res = await api.getMessages(id, {
        from,
        limit: MESSAGE_PAGE_SIZE,
        direction: "asc",
      });
      if (this.sessionId !== id) return;
      if (res.messages.length === 0) break;

      loaded = [...loaded, ...res.messages];
      this.messages = loaded;

      const newest = loaded[loaded.length - 1];
      this.messageCount = messageCountHint ??
        (newest ? newest.ordinal + 1 : loaded.length);
      this.hasOlder = false;

      if (res.messages.length < MESSAGE_PAGE_SIZE) break;
      const last = res.messages[res.messages.length - 1];
      if (!last) break;
      const nextFrom = last.ordinal + 1;
      if (nextFrom <= from) break;
      from = nextFrom;
    }

    const newest = this.messages[this.messages.length - 1];
    this.messageCount = messageCountHint ??
      (newest ? newest.ordinal + 1 : this.messages.length);
    this.hasOlder = false;
  }

  private async loadProgressively(id: string) {
    const firstRes = await api.getMessages(id, {
      limit: MESSAGE_PAGE_SIZE,
      direction: "desc",
    });

    if (this.sessionId !== id) return;
    // Keep in ascending ordinal order in store for simpler append
    // and stable ordinal math; UI handles newest-first presentation.
    this.messages = [...firstRes.messages].reverse();
    const newest = this.messages[this.messages.length - 1];
    this.messageCount = newest ? newest.ordinal + 1 : 0;
    const oldest = this.messages[0]?.ordinal;
    if (oldest !== undefined) {
      this.hasOlder = oldest > 0;
    } else {
      this.hasOlder = false;
    }
  }

  private async loadFrom(id: string, from: number) {
    for (;;) {
      if (this.sessionId !== id) return;

      const res = await api.getMessages(id, {
        from,
        limit: MESSAGE_PAGE_SIZE,
        direction: "asc",
      });

      if (this.sessionId !== id) return;
      if (res.messages.length === 0) break;

      this.messages.push(...res.messages);

      if (res.messages.length < MESSAGE_PAGE_SIZE) break;
      from =
        res.messages[res.messages.length - 1]!.ordinal + 1;
    }
  }

  async loadOlder() {
    if (
      !this.sessionId ||
      this.loadingOlder ||
      !this.hasOlder ||
      this.messages.length === 0
    ) return;
    const id = this.sessionId;
    const oldest = this.messages[0]!.ordinal;
    if (oldest <= 0) {
      this.hasOlder = false;
      return;
    }

    this.loadingOlder = true;
    try {
      const res = await api.getMessages(id, {
        from: oldest - 1,
        limit: MESSAGE_PAGE_SIZE,
        direction: "desc",
      });
      if (this.sessionId !== id) return;
      if (res.messages.length === 0) {
        this.hasOlder = false;
        return;
      }
      const chunk = [...res.messages].reverse();
      this.messages.unshift(...chunk);
      this.hasOlder = chunk[0]!.ordinal > 0;
    } finally {
      if (this.sessionId === id) {
        this.loadingOlder = false;
      }
    }
  }

  async ensureOrdinalLoaded(targetOrdinal: number) {
    if (!this.sessionId || this.messages.length === 0) return;

    const id = this.sessionId;
    const oldestLoaded = this.messages[0]!.ordinal;
    if (oldestLoaded <= targetOrdinal) return;
    if (!this.hasOlder) return;

    // If a scroll-triggered load is active, wait briefly for it
    // to finish before issuing targeted fetches.
    while (this.sessionId === id && this.loadingOlder) {
      await new Promise((r) => setTimeout(r, 16));
    }
    if (!this.sessionId || this.sessionId !== id) return;
    if (this.messages.length === 0) return;
    if (this.messages[0]!.ordinal <= targetOrdinal) return;

    this.loadingOlder = true;
    try {
      let from = this.messages[0]!.ordinal - 1;
      let lastOldest = this.messages[0]!.ordinal;
      const chunks: Message[][] = [];

      while (from >= 0) {
        if (this.sessionId !== id) return;
        const res = await api.getMessages(id, {
          from,
          limit: MESSAGE_PAGE_SIZE,
          direction: "desc",
        });
        if (this.sessionId !== id) return;
        if (res.messages.length === 0) {
          this.hasOlder = false;
          break;
        }

        const chunk = [...res.messages].reverse();
        chunks.push(chunk);
        const chunkOldest = chunk[0]!.ordinal;

        if (chunkOldest <= targetOrdinal) break;
        if (chunkOldest >= lastOldest) break;

        lastOldest = chunkOldest;
        from = chunkOldest - 1;
      }

      if (this.sessionId !== id) return;

      if (chunks.length > 0) {
        const merged = chunks.reverse().flat();
        this.messages = [...merged, ...this.messages];
      }

      const oldestNow = this.messages[0]?.ordinal;
      this.hasOlder = oldestNow !== undefined && oldestNow > 0;
    } catch {
      // Non-fatal: session may have changed or network error.
    } finally {
      if (this.sessionId === id) {
        this.loadingOlder = false;
      }
    }
  }

  private async reloadNow(id: string) {
    try {
      const sess = await api.getSession(id);
      if (this.sessionId !== id) return;

      const newCount = sess.message_count ?? 0;
      const oldCount = this.messageCount;
      if (newCount === oldCount) return;

      // Fast path: append only new messages.
      if (newCount > oldCount && this.messages.length > 0) {
        const lastOrdinal =
          this.messages[this.messages.length - 1]!.ordinal;
        await this.loadFrom(id, lastOrdinal + 1);
        if (this.sessionId !== id) return;

        // If incremental fetch fell out of sync, repair once.
        const newest = this.messages[this.messages.length - 1];
        if (newest && newest.ordinal !== newCount - 1) {
          await this.fullReload(id, newCount);
          return;
        }

        this.messageCount = newCount;
        return;
      }

      // Message count shrank (session rewrite) or we have no local
      // data yet: do a full reload.
      await this.fullReload(id, newCount);
    } catch {
      // Non-fatal. SSE watch should keep working and retry on the
      // next update tick.
    }
  }

  private async fullReload(
    id: string,
    messageCountHint?: number,
  ) {
    this.loading = true;
    try {
      if (
        messageCountHint !== undefined &&
        messageCountHint > FULL_SESSION_MESSAGE_THRESHOLD
      ) {
        await this.loadProgressively(id);
      } else {
        await this.loadAllMessages(id, messageCountHint);
      }
    } finally {
      if (this.sessionId === id) {
        this.loading = false;
      }
    }
  }
}

export const messages = new MessagesStore();
