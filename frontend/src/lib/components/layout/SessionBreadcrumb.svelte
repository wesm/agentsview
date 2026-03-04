<script lang="ts">
  import { onMount } from "svelte";
  import type { Session } from "../../api/types.js";
  import {
    resumeSession,
    listOpeners,
    openSession,
    type Opener,
  } from "../../api/client.js";
  import { copyToClipboard } from "../../utils/clipboard.js";
  import { agentColor } from "../../utils/agents.js";
  import {
    supportsResume,
    buildResumeCommand,
  } from "../../utils/resume.js";

  interface Props {
    session: Session | undefined;
    onBack: () => void;
  }

  let { session, onBack }: Props = $props();
  let copiedSessionId = $state("");
  let showOpenMenu = $state(false);
  let openers: Opener[] = $state([]);
  let openFeedback = $state("");
  let feedbackTimer: ReturnType<typeof setTimeout> | undefined;

  onMount(() => {
    listOpeners()
      .then((res) => { openers = res.openers; })
      .catch(() => {});
  });

  function sessionDisplayId(id: string): string {
    const idx = id.indexOf(":");
    return idx >= 0 ? id.slice(idx + 1) : id;
  }

  async function copySessionId(rawId: string, sessionId: string) {
    const ok = await copyToClipboard(rawId);
    if (!ok) return;
    copiedSessionId = sessionId;
    setTimeout(() => {
      if (copiedSessionId === sessionId) copiedSessionId = "";
    }, 1500);
  }

  function showFeedback(msg: string) {
    openFeedback = msg;
    clearTimeout(feedbackTimer);
    feedbackTimer = setTimeout(() => { openFeedback = ""; }, 2000);
  }

  async function handleOpen(opener: Opener) {
    if (!session) return;
    showOpenMenu = false;
    try {
      const resp = await openSession(session.id, opener.id);
      if (resp.launched) {
        showFeedback(`Opened in ${opener.name}`);
      }
    } catch {
      showFeedback("Failed to open");
    }
  }

  async function handleCopyPath() {
    if (!session) return;
    showOpenMenu = false;
    try {
      const resp = await resumeSession(session.id, { command_only: true });
      if (resp.cwd) {
        const ok = await copyToClipboard(resp.cwd);
        showFeedback(ok ? "Path copied!" : "Failed");
        return;
      }
    } catch {
      // Fall through to project field.
    }
    const fallback = session.project || "";
    if (fallback.startsWith("/")) {
      const ok = await copyToClipboard(fallback);
      showFeedback(ok ? "Path copied!" : "Failed");
    } else {
      showFeedback("No project path");
    }
  }

  async function handleCopyResumeCommand() {
    if (!session) return;
    showOpenMenu = false;
    try {
      const resp = await resumeSession(session.id, { command_only: true });
      if (resp.command) {
        const ok = await copyToClipboard(resp.command);
        showFeedback(ok ? "Command copied!" : "Failed");
        return;
      }
    } catch {
      // Fall back to local build.
    }
    const cmd = buildResumeCommand(session.agent, session.id);
    if (cmd) {
      const ok = await copyToClipboard(cmd);
      showFeedback(ok ? "Command copied!" : "Failed");
    } else {
      showFeedback("Not supported");
    }
  }

  const canResume = $derived(
    session ? supportsResume(session.agent) : false,
  );

  // Group openers by kind for display order.
  const fileOpeners = $derived(openers.filter((o) => o.kind === "files"));
  const editorOpeners = $derived(openers.filter((o) => o.kind === "editor"));
  const terminalOpeners = $derived(openers.filter((o) => o.kind === "terminal"));

  function handleClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement;
    if (!target.closest(".open-group")) {
      showOpenMenu = false;
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!showOpenMenu) return;
    if (e.key === "Escape") {
      showOpenMenu = false;
      e.preventDefault();
      return;
    }
    // Number key shortcuts (1-9) for quick selection.
    const num = parseInt(e.key);
    if (num >= 1 && num <= 9) {
      const all = [...fileOpeners, ...editorOpeners, ...terminalOpeners];
      const idx = num - 1;
      if (idx < all.length) {
        e.preventDefault();
        handleOpen(all[idx]);
      }
    }
  }
</script>

<svelte:window onclick={handleClickOutside} onkeydown={handleKeydown} />

<div class="session-breadcrumb">
  <button class="breadcrumb-link" onclick={onBack}>Sessions</button>
  <span class="breadcrumb-sep">/</span>
  <span class="breadcrumb-current">{session?.project ?? ""}</span>
  {#if session}
    <span class="breadcrumb-meta">
      <span
        class="agent-badge"
        style:background={agentColor(session.agent)}
      >{session.agent}</span>
      {#if session.started_at}
        <span class="session-time">
          {new Date(session.started_at).toLocaleDateString(undefined, {
            month: "short",
            day: "numeric",
          })}
          {new Date(session.started_at).toLocaleTimeString(undefined, {
            hour: "2-digit",
            minute: "2-digit",
          })}
        </span>
      {/if}
      <span class="open-group">
        <button
          class="open-btn"
          class:has-feedback={openFeedback !== ""}
          onclick={(e) => { e.stopPropagation(); showOpenMenu = !showOpenMenu; }}
          title="Open project in..."
          aria-label="Open project"
        >
          {#if openFeedback}
            <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z"/>
            </svg>
            {openFeedback}
          {:else}
            Open
            <svg width="8" height="8" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M4.427 7.427l3.396 3.396a.25.25 0 00.354 0l3.396-3.396A.25.25 0 0011.396 7H4.604a.25.25 0 00-.177.427z"/>
            </svg>
          {/if}
        </button>
        {#if showOpenMenu}
          {@const allOpeners = [...fileOpeners, ...editorOpeners, ...terminalOpeners]}
          <div class="open-menu">
            {#each allOpeners as opener, i (opener.id)}
              <button
                class="open-menu-item"
                onclick={() => handleOpen(opener)}
              >
                <span class="open-menu-num">{i + 1}</span>
                <span class="open-menu-name">{opener.name}</span>
                <span class="open-menu-kind">{opener.kind}</span>
              </button>
            {/each}
            {#if allOpeners.length > 0}
              <div class="open-menu-divider"></div>
            {/if}
            {#if canResume}
              <button class="open-menu-item" onclick={handleCopyResumeCommand}>
                <span class="open-menu-num">
                  <!-- terminal icon -->
                  <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor">
                    <path d="M0 2.75C0 1.784.784 1 1.75 1h12.5c.966 0 1.75.784 1.75 1.75v10.5A1.75 1.75 0 0114.25 15H1.75A1.75 1.75 0 010 13.25V2.75zm1.75-.25a.25.25 0 00-.25.25v10.5c0 .138.112.25.25.25h12.5a.25.25 0 00.25-.25V2.75a.25.25 0 00-.25-.25H1.75z"/>
                    <path d="M3.17 5.47a.75.75 0 011.06 0L6.53 7.77a.75.75 0 010 1.06L4.23 11.13a.75.75 0 01-1.06-1.06L5.44 7.8 3.17 6.53a.75.75 0 010-1.06zM7 10.25a.75.75 0 01.75-.75h3.5a.75.75 0 010 1.5h-3.5a.75.75 0 01-.75-.75z"/>
                  </svg>
                </span>
                <span class="open-menu-name">Resume command</span>
              </button>
            {/if}
            <button class="open-menu-item" onclick={handleCopyPath}>
              <span class="open-menu-num">
                <!-- copy icon -->
                <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M0 6.75C0 5.784.784 5 1.75 5h1.5a.75.75 0 010 1.5h-1.5a.25.25 0 00-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 00.25-.25v-1.5a.75.75 0 011.5 0v1.5A1.75 1.75 0 019.25 16h-7.5A1.75 1.75 0 010 14.25v-7.5z"/>
                  <path d="M5 1.75C5 .784 5.784 0 6.75 0h7.5C15.216 0 16 .784 16 1.75v7.5A1.75 1.75 0 0114.25 11h-7.5A1.75 1.75 0 015 9.25v-7.5zm1.75-.25a.25.25 0 00-.25.25v7.5c0 .138.112.25.25.25h7.5a.25.25 0 00.25-.25v-7.5a.25.25 0 00-.25-.25h-7.5z"/>
                </svg>
              </span>
              <span class="open-menu-name">Copy path</span>
            </button>
            {#if openers.length === 0}
              <div class="open-menu-empty">No applications detected</div>
            {/if}
          </div>
        {/if}
      </span>
      {#if session.id}
        {@const rawId = sessionDisplayId(session.id)}
        <button
          class="session-id"
          title={rawId}
          onclick={() => copySessionId(rawId, session.id)}
        >
          {copiedSessionId === session.id ? "Copied!" : rawId.slice(0, 8)}
        </button>
      {/if}
    </span>
  {/if}
</div>

<style>
  .session-breadcrumb {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 32px;
    padding: 0 14px;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
    font-size: 11px;
    color: var(--text-muted);
  }

  .breadcrumb-link {
    color: var(--text-muted);
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    transition: color 0.12s;
  }

  .breadcrumb-link:hover {
    color: var(--accent-blue);
  }

  .breadcrumb-sep {
    opacity: 0.3;
    font-size: 10px;
  }

  .breadcrumb-current {
    color: var(--text-primary);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
  }

  .breadcrumb-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-left: auto;
    flex-shrink: 0;
  }

  .agent-badge {
    font-size: 9px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 8px;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    color: white;
    flex-shrink: 0;
    background: var(--text-muted);
  }

  .session-time {
    font-size: 10px;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .open-group {
    position: relative;
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .open-btn {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 500;
    color: var(--text-muted);
    padding: 1px 8px;
    border-radius: 4px;
    background: var(--bg-tertiary);
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
    transition: color 0.15s, background 0.15s;
  }

  .open-btn:hover {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }

  .open-btn.has-feedback {
    color: var(--accent-green, #2ea043);
  }

  .open-menu {
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 4px;
    background: var(--bg-primary);
    border: 1px solid var(--border-default);
    border-radius: 8px;
    padding: 4px;
    min-width: 200px;
    z-index: 100;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
  }

  .open-menu-item {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 6px 10px;
    font-size: 13px;
    color: var(--text-primary);
    border-radius: 5px;
    cursor: pointer;
    transition: background 0.1s;
  }

  .open-menu-item:hover {
    background: var(--bg-hover);
  }

  .open-menu-num {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    font-size: 11px;
    font-weight: 500;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .open-menu-name {
    flex: 1;
    font-weight: 500;
  }

  .open-menu-kind {
    font-size: 9px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
    opacity: 0.6;
  }

  .open-menu-divider {
    height: 1px;
    background: var(--border-muted);
    margin: 4px 0;
  }

  .open-menu-empty {
    padding: 8px 10px;
    font-size: 11px;
    color: var(--text-muted);
    text-align: center;
  }

  .session-id {
    font-size: 10px;
    font-family: "SF Mono", "Menlo", "Consolas", monospace;
    color: var(--text-muted);
    cursor: pointer;
    padding: 1px 5px;
    border-radius: 4px;
    background: var(--bg-tertiary);
    transition: color 0.15s, background 0.15s;
    white-space: nowrap;
    flex-shrink: 0;
  }

  .session-id:hover {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }
</style>
