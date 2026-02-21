import * as api from "../api/client.js";
import type { Message } from "../api/types.js";

const MESSAGE_PAGE_SIZE = 1000;
const FULL_SESSION_MESSAGE_THRESHOLD = 20_000;

interface FetchPageOptions {
  from: number;
  limit: number;
  direction: "asc" | "desc";
  signal: AbortSignal;
}

class MessagesStore {
  messages: Message[] = $state([]);
  loading: boolean = $state(false);
  sessionId: string | null = $state(null);
  messageCount: number = $state(0);
  hasOlder: boolean = $state(false);
  loadingOlder: boolean = $state(false);
  private abortController: AbortController | null = null;
  private reloadPromise: Promise<void> | null = null;
  private reloadSessionId: string | null = null;
  private pendingReload: boolean = false;
  private loadOlderPromise: Promise<void> | null = null;

  async loadSession(id: string) {
    if (
      this.sessionId === id &&
      (this.messages.length > 0 || this.loading)
    ) {
      return;
    }
    this.clear();
    this.sessionId = id;
    this.loading = true;

    const ac = new AbortController();
    this.abortController = ac;

    try {
      let countHint: number | null = null;
      try {
        const sess = await api.getSession(id, {
          signal: ac.signal,
        });
        countHint = sess.message_count ?? 0;
      } catch (err) {
        if (isAbortError(err)) return;
        console.warn(
          "Failed to fetch session metadata:",
          err,
        );
      }

      if (
        countHint !== null &&
        countHint > FULL_SESSION_MESSAGE_THRESHOLD
      ) {
        await this.loadProgressively(id, ac.signal);
      } else {
        await this.loadAllMessages(
          id,
          ac.signal,
          countHint ?? undefined,
        );
      }
    } catch (err) {
      if (isAbortError(err)) return;
      console.warn("Failed to load session messages:", err);
    } finally {
      if (this.sessionId === id) {
        this.loading = false;
      }
    }
  }

  reload(): Promise<void> {
    if (!this.sessionId) return Promise.resolve();

    if (
      this.reloadPromise &&
      this.reloadSessionId === this.sessionId
    ) {
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
    this.abortController?.abort();
    this.abortController = null;
    this.messages = [];
    this.sessionId = null;
    this.loading = false;
    this.messageCount = 0;
    this.hasOlder = false;
    this.loadingOlder = false;
    this.reloadPromise = null;
    this.reloadSessionId = null;
    this.pendingReload = false;
    this.loadOlderPromise = null;
  }

  private async fetchPages(
    id: string,
    opts: FetchPageOptions,
  ): Promise<Message[]> {
    const loaded: Message[] = [];
    let from = opts.from;

    for (;;) {
      const res = await api.getMessages(
        id,
        { from, limit: opts.limit, direction: opts.direction },
        { signal: opts.signal },
      );
      if (res.messages.length === 0) break;

      loaded.push(...res.messages);

      if (res.messages.length < opts.limit) break;
      const last = res.messages[res.messages.length - 1];
      if (!last) break;

      const nextFrom =
        opts.direction === "asc"
          ? last.ordinal + 1
          : last.ordinal - 1;
      if (
        opts.direction === "asc"
          ? nextFrom <= from
          : nextFrom >= from
      ) {
        break;
      }
      from = nextFrom;
    }

    return loaded;
  }

  private async loadAllMessages(
    id: string,
    signal: AbortSignal,
    messageCountHint?: number,
  ) {
    let from = 0;
    let loaded: Message[] = [];

    for (;;) {
      const res = await api.getMessages(
        id,
        { from, limit: MESSAGE_PAGE_SIZE, direction: "asc" },
        { signal },
      );
      if (res.messages.length === 0) break;

      loaded = [...loaded, ...res.messages];
      this.messages = loaded;

      const newest = loaded[loaded.length - 1];
      this.messageCount =
        messageCountHint ??
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
    this.messageCount =
      messageCountHint ??
      (newest ? newest.ordinal + 1 : this.messages.length);
    this.hasOlder = false;
  }

  private async loadProgressively(
    id: string,
    signal: AbortSignal,
  ) {
    const firstRes = await api.getMessages(
      id,
      { limit: MESSAGE_PAGE_SIZE, direction: "desc" },
      { signal },
    );

    this.messages = [...firstRes.messages].reverse();
    const newest = this.messages[this.messages.length - 1];
    this.messageCount = newest ? newest.ordinal + 1 : 0;
    const oldest = this.messages[0]?.ordinal;
    this.hasOlder =
      oldest !== undefined ? oldest > 0 : false;
  }

  private async loadFrom(
    id: string,
    from: number,
    signal: AbortSignal,
  ) {
    const pages = await this.fetchPages(id, {
      from,
      limit: MESSAGE_PAGE_SIZE,
      direction: "asc",
      signal,
    });
    if (pages.length > 0) {
      this.messages.push(...pages);
    }
  }

  async loadOlder() {
    if (
      !this.sessionId ||
      this.loadOlderPromise ||
      !this.hasOlder ||
      this.messages.length === 0
    ) {
      return this.loadOlderPromise ?? undefined;
    }

    this.loadOlderPromise = this.doLoadOlder().finally(
      () => {
        this.loadOlderPromise = null;
      },
    );
    return this.loadOlderPromise;
  }

  private async doLoadOlder() {
    const id = this.sessionId;
    if (!id || this.messages.length === 0) return;

    const oldest = this.messages[0]!.ordinal;
    if (oldest <= 0) {
      this.hasOlder = false;
      return;
    }

    const signal = this.abortController?.signal;
    if (!signal || signal.aborted) return;

    this.loadingOlder = true;
    try {
      const res = await api.getMessages(
        id,
        {
          from: oldest - 1,
          limit: MESSAGE_PAGE_SIZE,
          direction: "desc",
        },
        { signal },
      );
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

    if (this.loadOlderPromise) {
      await this.loadOlderPromise;
    }
    if (!this.sessionId || this.sessionId !== id) return;
    if (this.messages.length === 0) return;
    if (this.messages[0]!.ordinal <= targetOrdinal) return;

    const signal = this.abortController?.signal;
    if (!signal || signal.aborted) return;

    this.loadingOlder = true;
    try {
      let from = this.messages[0]!.ordinal - 1;
      let lastOldest = this.messages[0]!.ordinal;
      const chunks: Message[][] = [];

      while (from >= 0) {
        const res = await api.getMessages(
          id,
          {
            from,
            limit: MESSAGE_PAGE_SIZE,
            direction: "desc",
          },
          { signal },
        );
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
      this.hasOlder =
        oldestNow !== undefined && oldestNow > 0;
    } catch (err) {
      if (isAbortError(err)) return;
      console.warn(
        "Failed to load older messages for ordinal:",
        err,
      );
    } finally {
      if (this.sessionId === id) {
        this.loadingOlder = false;
      }
    }
  }

  private async reloadNow(id: string) {
    const signal = this.abortController?.signal;
    if (!signal || signal.aborted) return;

    try {
      const sess = await api.getSession(id, { signal });
      if (this.sessionId !== id) return;

      const newCount = sess.message_count ?? 0;
      const oldCount = this.messageCount;
      if (newCount === oldCount) return;

      if (newCount > oldCount && this.messages.length > 0) {
        const lastOrdinal =
          this.messages[this.messages.length - 1]!.ordinal;
        await this.loadFrom(id, lastOrdinal + 1, signal);
        if (this.sessionId !== id) return;

        const newest =
          this.messages[this.messages.length - 1];
        if (newest && newest.ordinal !== newCount - 1) {
          await this.fullReload(id, signal, newCount);
          return;
        }

        this.messageCount = newCount;
        return;
      }

      await this.fullReload(id, signal, newCount);
    } catch (err) {
      if (isAbortError(err)) return;
      console.warn("Reload failed:", err);
    }
  }

  private async fullReload(
    id: string,
    signal: AbortSignal,
    messageCountHint?: number,
  ) {
    this.loading = true;
    try {
      if (
        messageCountHint !== undefined &&
        messageCountHint > FULL_SESSION_MESSAGE_THRESHOLD
      ) {
        await this.loadProgressively(id, signal);
      } else {
        await this.loadAllMessages(
          id,
          signal,
          messageCountHint,
        );
      }
    } finally {
      if (this.sessionId === id) {
        this.loading = false;
      }
    }
  }
}

function isAbortError(err: unknown): boolean {
  return (
    err instanceof DOMException && err.name === "AbortError"
  );
}

export const messages = new MessagesStore();
