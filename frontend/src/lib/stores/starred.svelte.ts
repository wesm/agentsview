import * as api from "../api/client.js";

const STORAGE_KEY = "agentsview-starred-sessions";

class StarredStore {
  ids: Set<string> = $state(new Set());
  filterOnly: boolean = $state(false);
  private loaded = false;
  private loading: Promise<void> | null = null;
  /** Global mutation counter for load/migration staleness detection. */
  private mutationVersion = 0;
  /** Monotonic counter for listStarred refresh calls so only the
   *  latest response applies when multiple are in-flight. */
  private refreshId = 0;
  /** Per-session promise chains to serialize server mutations. */
  private queues: Map<string, Promise<void>> = new Map();

  async load() {
    if (this.loaded) return;
    if (this.loading) return this.loading;
    this.loading = this.doLoad();
    return this.loading;
  }

  private async doLoad() {
    const mutVer = this.mutationVersion;
    const rid = ++this.refreshId;
    try {
      const res = await api.listStarred();
      if (this.mutationVersion === mutVer && this.refreshId === rid) {
        this.ids = new Set(res.session_ids);
      }
      this.loaded = true;
      await this.migrateLocalStorage();
    } catch {
      const local = readLocalStorage();
      if (local.size > 0 && this.mutationVersion === mutVer && this.refreshId === rid) {
        this.ids = local;
      }
    } finally {
      this.loading = null;
    }
  }

  private async migrateLocalStorage() {
    const local = readLocalStorage();
    if (local.size === 0) return;

    const toMigrate = [...local].filter((id) => !this.ids.has(id));
    if (toMigrate.length > 0) {
      const mutVer = this.mutationVersion;
      const rid = ++this.refreshId;
      try {
        await api.bulkStarSessions(toMigrate);
        const refreshed = await api.listStarred();
        if (this.mutationVersion === mutVer && this.refreshId === rid) {
          this.ids = new Set(refreshed.session_ids);
          clearLocalStorage();
        }
      } catch {
        // Migration failed — merge local IDs into memory so they
        // remain visible. localStorage is preserved for retry on
        // next page reload.
        const merged = new Set(this.ids);
        for (const id of toMigrate) merged.add(id);
        this.ids = merged;
        return;
      }
    } else {
      clearLocalStorage();
    }
  }

  isStarred(sessionId: string): boolean {
    return this.ids.has(sessionId);
  }

  toggle(sessionId: string) {
    if (this.ids.has(sessionId)) {
      this.unstar(sessionId);
    } else {
      this.star(sessionId);
    }
  }

  star(sessionId: string) {
    if (this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.add(sessionId);
    this.ids = next;
    this.mutationVersion++;
    this.enqueue(sessionId, () => api.starSession(sessionId));
  }

  unstar(sessionId: string) {
    if (!this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.delete(sessionId);
    this.ids = next;
    this.mutationVersion++;
    this.enqueue(sessionId, () => api.unstarSession(sessionId));
  }

  private enqueue(
    sessionId: string,
    op: () => Promise<unknown>,
  ) {
    const prev = this.queues.get(sessionId) ?? Promise.resolve();
    const chain: Promise<void> = prev
      .then(() => op(), () => op())
      .then(() => {}, () => {})
      .then(() => {
        if (this.queues.get(sessionId) === chain) {
          this.queues.delete(sessionId);
        }
        this.reconcileIfIdle();
      });
    this.queues.set(sessionId, chain);
  }

  /**
   * After all in-flight mutations settle, re-fetch server state
   * to correct any drift from failed requests. Uses refreshId to
   * ensure only the latest listStarred response is applied when
   * multiple refreshes are in-flight.
   */
  private reconcileIfIdle() {
    if (this.queues.size > 0) return;
    const mutVer = this.mutationVersion;
    const rid = ++this.refreshId;
    api.listStarred().then((res) => {
      if (this.mutationVersion === mutVer && this.refreshId === rid) {
        this.ids = new Set(res.session_ids);
      }
    }).catch(() => {
      // Server unavailable; keep optimistic state.
    });
  }

  get count(): number {
    return this.ids.size;
  }
}

function readLocalStorage(): Set<string> {
  try {
    const raw = localStorage?.getItem(STORAGE_KEY);
    if (raw) {
      const arr = JSON.parse(raw);
      if (Array.isArray(arr)) return new Set(arr);
    }
  } catch {
    // ignore
  }
  return new Set();
}

function clearLocalStorage() {
  try {
    localStorage?.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

export const starred = new StarredStore();
