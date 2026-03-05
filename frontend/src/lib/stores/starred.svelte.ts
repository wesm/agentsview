import * as api from "../api/client.js";

const STORAGE_KEY = "agentsview-starred-sessions";

class StarredStore {
  ids: Set<string> = $state(new Set());
  filterOnly: boolean = $state(false);
  private loaded = false;
  private loading: Promise<void> | null = null;
  /** Per-session version for rollback safety on API failure. */
  private opVersions: Map<string, number> = new Map();
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
      // Only apply if no mutations occurred during the fetch.
      if (this.mutationVersion === mutVer) {
        this.ids = new Set(res.session_ids);
      }
      this.loaded = true;
      // Best-effort migration after successful load.
      await this.migrateLocalStorage();
    } catch {
      // Server unavailable: fall back to localStorage only if
      // no mutations occurred during the fetch (otherwise the
      // user's optimistic state takes precedence).
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
        // Only apply if no user mutations happened during migration.
        if (this.mutationVersion === mutVer) {
          this.ids = new Set(refreshed.session_ids);
        }
      } catch {
        // Migration failed; don't clear localStorage so it retries.
        return;
      }
    }
    clearLocalStorage();
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

  private nextOpVersion(sessionId: string): number {
    const v = (this.opVersions.get(sessionId) ?? 0) + 1;
    this.opVersions.set(sessionId, v);
    return v;
  }

  star(sessionId: string) {
    if (this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.add(sessionId);
    this.ids = next;
    this.mutationVersion++;
    const version = this.nextOpVersion(sessionId);
    api.starSession(sessionId).catch(() => {
      if (this.opVersions.get(sessionId) !== version) return;
      const reverted = new Set(this.ids);
      reverted.delete(sessionId);
      this.ids = reverted;
    });
  }

  unstar(sessionId: string) {
    if (!this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.delete(sessionId);
    this.ids = next;
    this.mutationVersion++;
    const version = this.nextOpVersion(sessionId);
    api.unstarSession(sessionId).catch(() => {
      if (this.opVersions.get(sessionId) !== version) return;
      const reverted = new Set(this.ids);
      reverted.add(sessionId);
      this.ids = reverted;
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
