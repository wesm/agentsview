<script lang="ts">
  import type { Session } from "../../api/types.js";
  import { resumeSession } from "../../api/client.js";
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
  let resumeState: "idle" | "launching" | "launched" | "copied" | "error" =
    $state("idle");
  let resumeMessage = $state("");
  let showResumeMenu = $state(false);
  let skipPermissions = $state(false);

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

  function resetResumeState() {
    setTimeout(() => {
      resumeState = "idle";
      resumeMessage = "";
    }, 2500);
  }

  async function handleResumeLaunch() {
    if (!session) return;
    showResumeMenu = false;
    resumeState = "launching";

    try {
      const resp = await resumeSession(session.id, {
        skip_permissions: skipPermissions,
      });

      if (resp.launched) {
        resumeState = "launched";
        resumeMessage = resp.terminal
          ? `Launched in ${resp.terminal}`
          : "Launched!";
      } else {
        // Terminal not found — fall back to clipboard.
        const ok = await copyToClipboard(resp.command);
        if (ok) {
          resumeState = "copied";
          resumeMessage = resp.error
            ? "No terminal found — copied command"
            : "Copied!";
        } else {
          resumeState = "error";
          resumeMessage = "Failed to copy";
        }
      }
    } catch {
      // API error — fall back to building command locally.
      const cmd = buildResumeCommand(session.agent, session.id, {
        skipPermissions,
      });
      if (cmd) {
        const ok = await copyToClipboard(cmd);
        resumeState = ok ? "copied" : "error";
        resumeMessage = ok ? "Copied command" : "Failed";
      } else {
        resumeState = "error";
        resumeMessage = "Not supported";
      }
    }
    resetResumeState();
  }

  async function handleResumeCopy() {
    if (!session) return;
    showResumeMenu = false;
    const cmd = buildResumeCommand(session.agent, session.id, {
      skipPermissions,
    });
    if (!cmd) return;
    const ok = await copyToClipboard(cmd);
    resumeState = ok ? "copied" : "error";
    resumeMessage = ok ? "Copied!" : "Failed";
    resetResumeState();
  }

  const canResume = $derived(
    session ? supportsResume(session.agent) : false,
  );

  const isClaude = $derived(session?.agent === "claude");

  function handleClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement;
    if (!target.closest(".resume-group")) {
      showResumeMenu = false;
    }
  }
</script>

<svelte:window onclick={handleClickOutside} />

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
      {#if canResume}
        <span class="resume-group">
          <button
            class="resume-btn"
            class:launched={resumeState === "launched"}
            class:copied={resumeState === "copied"}
            class:error={resumeState === "error"}
            class:launching={resumeState === "launching"}
            onclick={handleResumeLaunch}
            title="Launch this session in a terminal"
            aria-label="Resume in terminal"
            disabled={resumeState === "launching"}
          >
            {#if resumeState === "launching"}
              <svg class="spin" width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                <path d="M8 1a7 7 0 100 14A7 7 0 008 1zm0 1.5a5.5 5.5 0 110 11 5.5 5.5 0 010-11z" opacity=".25"/>
                <path d="M8 1a7 7 0 017 7h-1.5A5.5 5.5 0 008 2.5V1z"/>
              </svg>
            {:else if resumeState === "launched"}
              <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                <path d="M13.78 4.22a.75.75 0 010 1.06l-7.25 7.25a.75.75 0 01-1.06 0L2.22 9.28a.75.75 0 011.06-1.06L6 10.94l6.72-6.72a.75.75 0 011.06 0z"/>
              </svg>
            {:else if resumeState === "copied"}
              <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                <path d="M0 2.75C0 1.784.784 1 1.75 1h8.5c.966 0 1.75.784 1.75 1.75v7.5A1.75 1.75 0 0110.25 12h-8.5A1.75 1.75 0 010 10.25v-7.5zm1.75-.25a.25.25 0 00-.25.25v7.5c0 .138.112.25.25.25h8.5a.25.25 0 00.25-.25v-7.5a.25.25 0 00-.25-.25h-8.5z"/>
                <path d="M3.5 4.5h5v1h-5zm0 2h5v1h-5zm0 2h3v1h-3z"/>
              </svg>
            {:else}
              <!-- Terminal icon -->
              <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                <path d="M0 2.75C0 1.784.784 1 1.75 1h12.5c.966 0 1.75.784 1.75 1.75v10.5A1.75 1.75 0 0114.25 15H1.75A1.75 1.75 0 010 13.25V2.75zm1.75-.25a.25.25 0 00-.25.25v10.5c0 .138.112.25.25.25h12.5a.25.25 0 00.25-.25V2.75a.25.25 0 00-.25-.25H1.75z"/>
                <path d="M3.17 5.47a.75.75 0 011.06 0L6.53 7.77a.75.75 0 010 1.06L4.23 11.13a.75.75 0 01-1.06-1.06L5.44 7.8 3.17 6.53a.75.75 0 010-1.06zM7 10.25a.75.75 0 01.75-.75h3.5a.75.75 0 010 1.5h-3.5a.75.75 0 01-.75-.75z"/>
              </svg>
            {/if}
            {resumeMessage || "Resume"}
          </button>
          <button
            class="resume-dropdown-trigger"
            onclick={(e) => { e.stopPropagation(); showResumeMenu = !showResumeMenu; }}
            title="Resume options"
            aria-label="Resume options"
          >
            <svg width="8" height="8" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
              <path d="M4.427 7.427l3.396 3.396a.25.25 0 00.354 0l3.396-3.396A.25.25 0 0011.396 7H4.604a.25.25 0 00-.177.427z"/>
            </svg>
          </button>
          {#if showResumeMenu}
            <div class="resume-menu">
              <button class="resume-menu-item" onclick={handleResumeLaunch}>
                <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M0 2.75C0 1.784.784 1 1.75 1h12.5c.966 0 1.75.784 1.75 1.75v10.5A1.75 1.75 0 0114.25 15H1.75A1.75 1.75 0 010 13.25V2.75zm1.75-.25a.25.25 0 00-.25.25v10.5c0 .138.112.25.25.25h12.5a.25.25 0 00.25-.25V2.75a.25.25 0 00-.25-.25H1.75z"/>
                  <path d="M3.17 5.47a.75.75 0 011.06 0L6.53 7.77a.75.75 0 010 1.06L4.23 11.13a.75.75 0 01-1.06-1.06L5.44 7.8 3.17 6.53a.75.75 0 010-1.06z"/>
                </svg>
                Launch in terminal
              </button>
              <button class="resume-menu-item" onclick={handleResumeCopy}>
                <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M0 2.75C0 1.784.784 1 1.75 1h8.5c.966 0 1.75.784 1.75 1.75v7.5A1.75 1.75 0 0110.25 12h-8.5A1.75 1.75 0 010 10.25v-7.5zm1.75-.25a.25.25 0 00-.25.25v7.5c0 .138.112.25.25.25h8.5a.25.25 0 00.25-.25v-7.5a.25.25 0 00-.25-.25h-8.5z"/>
                  <path d="M3.5 4.5h5v1h-5zm0 2h5v1h-5zm0 2h3v1h-3z"/>
                </svg>
                Copy command
              </button>
              {#if isClaude}
                <div class="resume-menu-divider"></div>
                <label class="resume-menu-toggle">
                  <input type="checkbox" bind:checked={skipPermissions} />
                  Skip permissions
                </label>
              {/if}
            </div>
          {/if}
        </span>
      {/if}
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

  .resume-group {
    position: relative;
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .resume-btn {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 10px;
    font-weight: 500;
    color: var(--text-muted);
    padding: 1px 7px;
    border-radius: 4px 0 0 4px;
    background: var(--bg-tertiary);
    cursor: pointer;
    white-space: nowrap;
    flex-shrink: 0;
    transition: color 0.15s, background 0.15s;
  }

  .resume-btn:hover {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }

  .resume-btn.launched {
    color: var(--accent-green, #2ea043);
  }

  .resume-btn.copied {
    color: var(--accent-blue, #58a6ff);
  }

  .resume-btn.error {
    color: var(--accent-red, #f85149);
  }

  .resume-btn.launching {
    opacity: 0.7;
    cursor: wait;
  }

  .resume-btn:disabled {
    pointer-events: none;
  }

  .resume-dropdown-trigger {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 1px 4px;
    border-radius: 0 4px 4px 0;
    background: var(--bg-tertiary);
    color: var(--text-muted);
    cursor: pointer;
    border-left: 1px solid var(--border-muted);
    transition: color 0.15s, background 0.15s;
  }

  .resume-dropdown-trigger:hover {
    color: var(--text-secondary);
    background: var(--bg-hover);
  }

  .resume-menu {
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 4px;
    background: var(--bg-primary);
    border: 1px solid var(--border-default);
    border-radius: 6px;
    padding: 4px;
    min-width: 170px;
    z-index: 100;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  }

  .resume-menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 5px 8px;
    font-size: 11px;
    color: var(--text-secondary);
    border-radius: 4px;
    cursor: pointer;
    transition: background 0.1s;
  }

  .resume-menu-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .resume-menu-divider {
    height: 1px;
    background: var(--border-muted);
    margin: 4px 0;
  }

  .resume-menu-toggle {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 8px;
    font-size: 11px;
    color: var(--text-secondary);
    cursor: pointer;
    user-select: none;
  }

  .resume-menu-toggle input {
    accent-color: var(--accent-blue);
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

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .spin {
    animation: spin 0.8s linear infinite;
  }
</style>
