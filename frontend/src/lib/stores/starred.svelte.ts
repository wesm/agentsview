import * as api from "../api/client.js";

const STORAGE_KEY = "agentsview-starred-sessions";

class StarredStore {
  ids: Set<string> = $state(new Set());
  filterOnly: boolean = $state(false);
  private loaded: boolean = false;
  private loading: Promise<void> | null = null;
  private opVersions: Map<string, number> = new Map();
  private loadVersion: number = 0;

  /** Load starred IDs from the server. Migrates localStorage data on first load. */
  async load() {
    if (this.loaded) return;
    if (this.loading) return this.loading;
    this.loading = this.doLoad();
    return this.loading;
  }

  private async doLoad() {
    try {
      const vBefore = this.loadVersion;
      const res = await api.listStarred();
      if (this.loadVersion === vBefore) {
        const merged = new Set(res.session_ids);
        for (const id of this.ids) merged.add(id);
        this.ids = merged;
      }

      await this.migrateLocalStorage();
      this.loaded = true;
    } catch {
      const local = readLocalStorage();
      const merged = new Set(local);
      for (const id of this.ids) merged.add(id);
      this.ids = merged;
    } finally {
      this.loading = null;
    }
  }

  private async migrateLocalStorage() {
    const local = readLocalStorage();
    if (local.size === 0) return;

    // Find IDs in localStorage that aren't already in the DB
    const toMigrate = [...local].filter((id) => !this.ids.has(id));
    if (toMigrate.length > 0) {
      await api.bulkStarSessions(toMigrate);
      const refreshed = await api.listStarred();
      const refreshedSet = new Set(refreshed.session_ids);
      const next = new Set(this.ids);
      for (const id of toMigrate) {
        if (refreshedSet.has(id)) next.add(id);
      }
      this.ids = next;
    }

    // Migration succeeded — clear localStorage
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

  private nextVersion(sessionId: string): number {
    const v = (this.opVersions.get(sessionId) ?? 0) + 1;
    this.opVersions.set(sessionId, v);
    this.loadVersion++;
    return v;
  }

  star(sessionId: string) {
    if (this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.add(sessionId);
    this.ids = next;
    // Track version so stale failures don't revert newer actions.
    const version = this.nextVersion(sessionId);
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
    const version = this.nextVersion(sessionId);
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
