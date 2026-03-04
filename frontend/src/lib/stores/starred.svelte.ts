import * as api from "../api/client.js";

const STORAGE_KEY = "agentsview-starred-sessions";

class StarredStore {
  ids: Set<string> = $state(new Set());
  filterOnly: boolean = $state(false);
  private loaded: boolean = false;

  /** Load starred IDs from the server. Migrates localStorage data on first load. */
  async load() {
    if (this.loaded) return;
    try {
      const res = await api.listStarred();
      this.ids = new Set(res.session_ids);
      this.loaded = true;

      // Migrate any localStorage stars to the database
      await this.migrateLocalStorage();
    } catch {
      // Fallback: read from localStorage if server unreachable
      this.ids = readLocalStorage();
      this.loaded = true;
    }
  }

  private async migrateLocalStorage() {
    const local = readLocalStorage();
    if (local.size === 0) return;

    // Find IDs in localStorage that aren't already in the DB
    const toMigrate = [...local].filter((id) => !this.ids.has(id));
    if (toMigrate.length > 0) {
      try {
        await api.bulkStarSessions(toMigrate);
        const next = new Set(this.ids);
        for (const id of toMigrate) next.add(id);
        this.ids = next;
      } catch {
        // Migration failed silently — will retry next load
        return;
      }
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

  star(sessionId: string) {
    if (this.ids.has(sessionId)) return;
    // Optimistic update
    const next = new Set(this.ids);
    next.add(sessionId);
    this.ids = next;
    // Fire and forget — revert on error
    api.starSession(sessionId).catch(() => {
      const reverted = new Set(this.ids);
      reverted.delete(sessionId);
      this.ids = reverted;
    });
  }

  unstar(sessionId: string) {
    if (!this.ids.has(sessionId)) return;
    // Optimistic update
    const next = new Set(this.ids);
    next.delete(sessionId);
    this.ids = next;
    // Fire and forget — revert on error
    api.unstarSession(sessionId).catch(() => {
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
