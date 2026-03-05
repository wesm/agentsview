import * as api from "../api/client.js";

const STORAGE_KEY = "agentsview-starred-sessions";

class StarredStore {
  ids: Set<string> = $state(new Set());
  filterOnly: boolean = $state(false);
  private loaded = false;
  private loading: Promise<void> | null = null;
  /** Global mutation counter for load/migration staleness detection. */
  private mutationVersion = 0;

  async load() {
    if (this.loaded) return;
    if (this.loading) return this.loading;
    this.loading = this.doLoad();
    return this.loading;
  }

  private async doLoad() {
    const mutVer = this.mutationVersion;
    try {
      const res = await api.listStarred();
      if (this.mutationVersion === mutVer) {
        this.ids = new Set(res.session_ids);
      }
      this.loaded = true;
      await this.migrateLocalStorage();
    } catch {
      const local = readLocalStorage();
      if (local.size > 0 && this.mutationVersion === mutVer) {
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
      try {
        await api.bulkStarSessions(toMigrate);
        const refreshed = await api.listStarred();
        if (this.mutationVersion === mutVer) {
          this.ids = new Set(refreshed.session_ids);
          clearLocalStorage();
        }
        // mutVer mismatch: keep localStorage so migration retries
        // on next page load once in-memory state is authoritative.
      } catch {
        return;
      }
    } else {
      // All local stars already present in server state.
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
    api.starSession(sessionId).catch(() => {
      this.reconcileAfterError();
    });
  }

  unstar(sessionId: string) {
    if (!this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.delete(sessionId);
    this.ids = next;
    this.mutationVersion++;
    api.unstarSession(sessionId).catch(() => {
      this.reconcileAfterError();
    });
  }

  private reconcileAfterError() {
    const mutVer = this.mutationVersion;
    api.listStarred().then((res) => {
      if (this.mutationVersion === mutVer) {
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
