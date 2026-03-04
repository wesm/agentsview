const STORAGE_KEY = "agentsview-starred-sessions";

function readStarred(): Set<string> {
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

function persist(ids: Set<string>) {
  try {
    localStorage?.setItem(
      STORAGE_KEY,
      JSON.stringify([...ids]),
    );
  } catch {
    // ignore
  }
}

class StarredStore {
  ids: Set<string> = $state(readStarred());
  filterOnly: boolean = $state(false);

  isStarred(sessionId: string): boolean {
    return this.ids.has(sessionId);
  }

  toggle(sessionId: string) {
    const next = new Set(this.ids);
    if (next.has(sessionId)) {
      next.delete(sessionId);
    } else {
      next.add(sessionId);
    }
    this.ids = next;
    persist(next);
  }

  star(sessionId: string) {
    if (this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.add(sessionId);
    this.ids = next;
    persist(next);
  }

  unstar(sessionId: string) {
    if (!this.ids.has(sessionId)) return;
    const next = new Set(this.ids);
    next.delete(sessionId);
    this.ids = next;
    persist(next);
  }

  get count(): number {
    return this.ids.size;
  }
}

export const starred = new StarredStore();
