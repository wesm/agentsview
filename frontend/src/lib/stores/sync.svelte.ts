import * as api from "../api/client.js";
import type {
  SyncProgress,
  SyncStats,
  Stats,
  VersionInfo,
} from "../api/types.js";

const POLL_INTERVAL_MS = 10_000;
const HASH_PREFIX_LEN = 7;

/**
 * Compare two commit hashes, tolerating short vs full SHA.
 * Returns true when both are known and they disagree.
 * Uses prefix comparison only when one hash is shorter
 * than the other (i.e. an abbreviation).
 */
export function commitsDisagree(
  a: string,
  b: string,
): boolean {
  if (a === "unknown" || b === "unknown") return false;
  if (a === b) return false;
  const minLen = Math.min(a.length, b.length);
  if (minLen < HASH_PREFIX_LEN) return true;
  if (a.length !== b.length) {
    return a.slice(0, minLen) !== b.slice(0, minLen);
  }
  return true;
}

class SyncStore {
  syncing: boolean = $state(false);
  progress: SyncProgress | null = $state(null);
  lastSync: string | null = $state(null);
  lastSyncStats: SyncStats | null = $state(null);
  stats: Stats | null = $state(null);
  serverVersion: VersionInfo | null = $state(null);
  versionMismatch: boolean = $state(false);
  readonly buildCommit: string =
    import.meta.env.VITE_BUILD_COMMIT;

  private watchEventSource: EventSource | null = null;
  private pollTimer: ReturnType<typeof setInterval> | null =
    null;

  async loadStatus() {
    try {
      const status = await api.getSyncStatus();
      const newLastSync = status.last_sync || null;
      const changed =
        newLastSync !== null && newLastSync !== this.lastSync;
      this.lastSync = newLastSync;
      this.lastSyncStats = status.stats;
      if (changed) this.loadStats();
    } catch {
      // Non-fatal
    }
  }

  startPolling() {
    this.stopPolling();
    this.pollTimer = setInterval(
      () => this.loadStatus(),
      POLL_INTERVAL_MS,
    );
  }

  stopPolling() {
    if (this.pollTimer) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  async loadStats() {
    try {
      this.stats = await api.getStats();
    } catch {
      // Non-fatal
    }
  }

  async loadVersion() {
    try {
      this.serverVersion = await api.getVersion();
      this.versionMismatch = commitsDisagree(
        this.buildCommit,
        this.serverVersion.commit,
      );
    } catch {
      // Non-fatal
    }
  }

  triggerSync(onComplete?: () => void) {
    if (this.syncing) return;
    this.syncing = true;
    this.progress = null;

    api.triggerSync({
      onProgress: (p: SyncProgress) => {
        this.progress = p;
      },
      onDone: (s: SyncStats) => {
        this.lastSyncStats = s;
        this.lastSync = new Date().toISOString();
        this.syncing = false;
        this.progress = null;
        this.loadStats();
        onComplete?.();
      },
      onError: () => {
        this.syncing = false;
        this.progress = null;
      },
    });
  }

  watchSession(sessionId: string, onUpdate: () => void) {
    this.unwatchSession();
    this.watchEventSource = api.watchSession(
      sessionId,
      onUpdate,
    );
  }

  unwatchSession() {
    if (this.watchEventSource) {
      this.watchEventSource.close();
      this.watchEventSource = null;
    }
  }
}

export const sync = new SyncStore();
