<script lang="ts">
  import { ui } from "../../stores/ui.svelte.js";
  import { sync } from "../../stores/sync.svelte.js";
  import { sessions } from "../../stores/sessions.svelte.js";

  type View = "confirm" | "progress" | "done" | "error";

  let view: View = $state("confirm");
  let errorMessage: string = $state("");

  function startResync() {
    const started = sync.triggerResync(
      () => {
        view = "done";
        sessions.load();
      },
      (err) => {
        errorMessage = err.message;
        view = "error";
      },
    );
    if (started) {
      view = "progress";
    } else {
      errorMessage = "A sync is already in progress.";
      view = "error";
    }
  }

  function close() {
    ui.activeModal = null;
  }

  function handleOverlayClick(e: MouseEvent) {
    if (
      view !== "progress" &&
      (e.target as HTMLElement).classList.contains(
        "resync-overlay",
      )
    ) {
      close();
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && view !== "progress") {
      close();
    }
  }

  const progressPct = $derived(
    sync.progress
      ? sync.progress.sessions_total > 0
        ? (sync.progress.sessions_done /
            sync.progress.sessions_total) *
          100
        : 0
      : 0,
  );
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="resync-overlay"
  onclick={handleOverlayClick}
  onkeydown={handleKeydown}
>
  <div class="resync-modal">
    <div class="modal-header">
      <h3 class="modal-title">Full Resync</h3>
      {#if view !== "progress"}
        <button class="close-btn" onclick={close}>
          &times;
        </button>
      {/if}
    </div>

    <div class="modal-body">
      {#if view === "confirm"}
        <p class="confirm-text">
          Re-parse all session files from scratch. Existing
          sessions will be updated in place &mdash; no data is
          deleted. Use this after upgrading or when sessions
          appear incorrect.
        </p>
        <div class="confirm-actions">
          <button class="btn" onclick={close}>
            Cancel
          </button>
          <button
            class="btn btn-primary"
            onclick={startResync}
          >
            Start Full Resync
          </button>
        </div>

      {:else if view === "progress"}
        <div class="progress-view">
          <div class="spinner"></div>
          <p class="progress-label">
            {#if sync.progress}
              Syncing {sync.progress.sessions_done}
              / {sync.progress.sessions_total} sessions...
            {:else}
              Preparing...
            {/if}
          </p>
          <div class="progress-bar-track">
            <div
              class="progress-bar-fill"
              style="width: {progressPct}%"
            ></div>
          </div>
        </div>

      {:else if view === "done"}
        <div class="done-view">
          {#if sync.lastSyncStats}
            <div class="stats-grid">
              <span class="stat-label">Synced</span>
              <span class="stat-value">
                {sync.lastSyncStats.synced}
              </span>
              <span class="stat-label">Skipped</span>
              <span class="stat-value">
                {sync.lastSyncStats.skipped}
              </span>
              <span class="stat-label">Total</span>
              <span class="stat-value">
                {sync.lastSyncStats.total_sessions}
              </span>
            </div>
          {/if}
          <div class="done-actions">
            <button class="btn btn-primary" onclick={close}>
              Close
            </button>
          </div>
        </div>

      {:else if view === "error"}
        <div class="error-view">
          <p class="error-message">{errorMessage}</p>
          <div class="error-actions">
            <button class="btn btn-primary" onclick={startResync}>
              Retry
            </button>
            <button class="btn" onclick={close}>
              Close
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .resync-overlay {
    position: fixed;
    inset: 0;
    background: var(--overlay-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .resync-modal {
    width: 400px;
    background: var(--bg-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
    overflow: hidden;
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border-default);
  }

  .modal-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .close-btn {
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 16px;
    color: var(--text-muted);
    border-radius: var(--radius-sm);
  }

  .close-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .modal-body {
    padding: 16px;
  }

  .confirm-text {
    font-size: 12px;
    color: var(--text-secondary);
    line-height: 1.5;
    margin-bottom: 16px;
  }

  .confirm-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .progress-view {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 16px 0;
  }

  .spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--border-default);
    border-top-color: var(--accent-blue);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .progress-label {
    font-size: 12px;
    color: var(--text-secondary);
    font-variant-numeric: tabular-nums;
  }

  .progress-bar-track {
    width: 100%;
    height: 4px;
    background: var(--bg-inset);
    border-radius: 2px;
    overflow: hidden;
  }

  .progress-bar-fill {
    height: 100%;
    background: var(--accent-blue);
    border-radius: 2px;
    transition: width 0.3s;
  }

  .done-view {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .stats-grid {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 4px 12px;
    font-size: 12px;
  }

  .stat-label {
    color: var(--text-muted);
    font-weight: 500;
  }

  .stat-value {
    color: var(--text-primary);
    font-variant-numeric: tabular-nums;
  }

  .done-actions {
    display: flex;
    justify-content: flex-end;
  }

  .error-view {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .error-message {
    font-size: 12px;
    color: var(--accent-red, #f85149);
    background: var(--bg-inset);
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--accent-red, #f85149);
    word-break: break-word;
  }

  .error-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .btn {
    height: 28px;
    padding: 0 12px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    background: var(--bg-surface-hover);
    color: var(--text-secondary);
    border: 1px solid var(--border-default);
  }

  .btn:hover {
    background: var(--bg-inset);
    color: var(--text-primary);
  }

  .btn-primary {
    background: var(--accent-blue);
    color: white;
    border-color: var(--accent-blue);
  }

  .btn-primary:hover {
    opacity: 0.9;
  }
</style>
