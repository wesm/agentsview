<script lang="ts">
  import type { Session } from "../../api/types.js";
  import {
    resumeSession,
    getTerminalConfig,
    setTerminalConfig,
    type TerminalConfig,
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
  let resumeState: "idle" | "launching" | "launched" | "copied" | "error" =
    $state("idle");
  let resumeMessage = $state("");
  let showResumeMenu = $state(false);
  let skipPermissions = $state(false);
  let showSettings = $state(false);
  let termMode: TerminalConfig["mode"] = $state("auto");
  let termCustomBin = $state("");
  let termCustomArgs = $state("");
  let settingsSaved = $state(false);
  let settingsError = $state("");

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

  async function handleResumeCopy() {
    if (!session) return;
    showResumeMenu = false;

    // Try the backend first — it builds the full command including
    // cd to the project directory. Fall back to local build.
    try {
      const resp = await resumeSession(session.id, {
        skip_permissions: skipPermissions,
      });
      const cmd = resp.command;
      if (cmd) {
        const ok = await copyToClipboard(cmd);
        resumeState = ok ? "copied" : "error";
        resumeMessage = ok ? "Copied!" : "Failed";
        resetResumeState();
        return;
      }
    } catch {
      // Backend unavailable — build locally.
    }

    const cmd = buildResumeCommand(session.agent, session.id, {
      skipPermissions,
    });
    if (!cmd) return;
    const ok = await copyToClipboard(cmd);
    resumeState = ok ? "copied" : "error";
    resumeMessage = ok ? "Copied!" : "Failed";
    resetResumeState();
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
          ? `Launched in ${resp.terminal.split("/").pop()}`
          : "Launched!";
      } else {
        // Terminal not found — copy instead.
        const ok = await copyToClipboard(resp.command);
        resumeState = ok ? "copied" : "error";
        resumeMessage = ok ? "No terminal — copied" : "Failed";
      }
    } catch {
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

  async function openSettings() {
    showResumeMenu = false;
    showSettings = true;
    settingsSaved = false;
    settingsError = "";
    try {
      const cfg = await getTerminalConfig();
      termMode = cfg.mode || "auto";
      termCustomBin = cfg.custom_bin || "";
      termCustomArgs = cfg.custom_args || "";
    } catch {
      // Defaults are fine.
    }
  }

  async function saveSettings() {
    settingsError = "";
    try {
      await setTerminalConfig({
        mode: termMode,
        custom_bin: termCustomBin || undefined,
        custom_args: termCustomArgs || undefined,
      });
      settingsSaved = true;
      setTimeout(() => {
        showSettings = false;
        settingsSaved = false;
      }, 800);
    } catch (err: unknown) {
      const raw = err instanceof Error ? err.message : String(err);
      try {
        const body = JSON.parse(raw);
        settingsError = body.error || raw;
      } catch {
        settingsError = raw;
      }
    }
  }

  const canResume = $derived(
    session ? supportsResume(session.agent) : false,
  );

  const isClaude = $derived(session?.agent === "claude");

  function handleClickOutside(e: MouseEvent) {
    const target = e.target as HTMLElement;
    if (!target.closest(".resume-group") && !target.closest(".terminal-settings")) {
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
            onclick={handleResumeCopy}
            title="Copy resume command to clipboard"
            aria-label="Copy resume command"
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
              <!-- Clipboard icon -->
              <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                <path d="M0 2.75C0 1.784.784 1 1.75 1h8.5c.966 0 1.75.784 1.75 1.75v7.5A1.75 1.75 0 0110.25 12h-8.5A1.75 1.75 0 010 10.25v-7.5zm1.75-.25a.25.25 0 00-.25.25v7.5c0 .138.112.25.25.25h8.5a.25.25 0 00.25-.25v-7.5a.25.25 0 00-.25-.25h-8.5z"/>
                <path d="M3.5 4.5h5v1h-5zm0 2h5v1h-5zm0 2h3v1h-3z"/>
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
              <button class="resume-menu-item" onclick={handleResumeCopy}>
                <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M0 2.75C0 1.784.784 1 1.75 1h8.5c.966 0 1.75.784 1.75 1.75v7.5A1.75 1.75 0 0110.25 12h-8.5A1.75 1.75 0 010 10.25v-7.5zm1.75-.25a.25.25 0 00-.25.25v7.5c0 .138.112.25.25.25h8.5a.25.25 0 00.25-.25v-7.5a.25.25 0 00-.25-.25h-8.5z"/>
                  <path d="M3.5 4.5h5v1h-5zm0 2h5v1h-5zm0 2h3v1h-3z"/>
                </svg>
                Copy command
              </button>
              <button class="resume-menu-item" onclick={handleResumeLaunch}>
                <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M0 2.75C0 1.784.784 1 1.75 1h12.5c.966 0 1.75.784 1.75 1.75v10.5A1.75 1.75 0 0114.25 15H1.75A1.75 1.75 0 010 13.25V2.75zm1.75-.25a.25.25 0 00-.25.25v10.5c0 .138.112.25.25.25h12.5a.25.25 0 00.25-.25V2.75a.25.25 0 00-.25-.25H1.75z"/>
                  <path d="M3.17 5.47a.75.75 0 011.06 0L6.53 7.77a.75.75 0 010 1.06L4.23 11.13a.75.75 0 01-1.06-1.06L5.44 7.8 3.17 6.53a.75.75 0 010-1.06z"/>
                </svg>
                Open in terminal
              </button>
              {#if isClaude}
                <div class="resume-menu-divider"></div>
                <label class="resume-menu-toggle">
                  <input type="checkbox" bind:checked={skipPermissions} />
                  Skip permissions
                </label>
              {/if}
              <div class="resume-menu-divider"></div>
              <button class="resume-menu-item" onclick={openSettings}>
                <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M8 4a.5.5 0 01.5.5v3h3a.5.5 0 010 1h-3v3a.5.5 0 01-1 0v-3h-3a.5.5 0 010-1h3v-3A.5.5 0 018 4z" opacity="0"/>
                  <path fill-rule="evenodd" d="M7.429 1.525a3.5 3.5 0 011.142 0c.036.003.108.036.137.146l.289 1.105c.147.56.55.967.997 1.189.174.086.326.183.48.276l.04.024c.162.097.35.182.569.231l1.113.267c.11.027.163.085.186.117a3.5 3.5 0 01.571.99c.014.04.02.123-.06.207l-.826.838a2.1 2.1 0 00-.554 1.087 3.3 3.3 0 010 .56 2.1 2.1 0 00.554 1.088l.826.837c.08.085.074.168.06.208a3.5 3.5 0 01-.571.99c-.023.031-.076.09-.186.117l-1.113.268a2.1 2.1 0 00-.57.231l-.04.024a3 3 0 01-.479.276c-.447.222-.85.628-.997 1.189l-.29 1.105c-.028.11-.1.143-.136.146a3.5 3.5 0 01-1.142 0c-.036-.003-.108-.037-.137-.146l-.289-1.105a2.1 2.1 0 00-.997-1.189 3 3 0 01-.48-.276l-.04-.024a2.1 2.1 0 00-.569-.231l-1.113-.268c-.11-.027-.163-.085-.186-.117a3.5 3.5 0 01-.571-.99c-.014-.04-.02-.123.06-.207l.826-.838A2.1 2.1 0 003.82 8.28a3.3 3.3 0 010-.56A2.1 2.1 0 003.266 6.63l-.826-.837a.18.18 0 01-.06-.208 3.5 3.5 0 01.571-.99c.023-.031.076-.09.186-.117l1.113-.268a2.1 2.1 0 00.57-.231l.04-.024c.162-.097.317-.19.479-.276a2.1 2.1 0 00.997-1.189l.29-1.105c.028-.11.1-.143.136-.146zM8 10.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5z"/>
                </svg>
                Terminal settings
              </button>
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

{#if showSettings}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="modal-overlay" onclick={() => (showSettings = false)}>
    <div class="terminal-settings modal-panel" onclick={(e) => e.stopPropagation()}>
      <div class="modal-header">
        <span class="modal-title">Terminal Settings</span>
        <button class="modal-close" onclick={() => (showSettings = false)}>&times;</button>
      </div>
      <div class="modal-body">
        <div class="settings-field">
          <span class="settings-label">Launch mode</span>
          <div class="settings-radios">
            <label class="settings-radio">
              <input type="radio" name="term-mode" value="auto" bind:group={termMode} />
              <span>Auto-detect</span>
              <span class="settings-hint">Find terminal on server</span>
            </label>
            <label class="settings-radio">
              <input type="radio" name="term-mode" value="clipboard" bind:group={termMode} />
              <span>Clipboard only</span>
              <span class="settings-hint">Always copy command, never launch</span>
            </label>
            <label class="settings-radio">
              <input type="radio" name="term-mode" value="custom" bind:group={termMode} />
              <span>Custom terminal</span>
              <span class="settings-hint">Specify binary and arguments</span>
            </label>
          </div>
        </div>
        {#if termMode === "custom"}
          <div class="settings-field">
            <label class="settings-label" for="term-bin">Terminal binary</label>
            <input
              id="term-bin"
              class="settings-input"
              type="text"
              placeholder="/usr/bin/kitty"
              bind:value={termCustomBin}
            />
          </div>
          <div class="settings-field">
            <label class="settings-label" for="term-args">Arguments template</label>
            <input
              id="term-args"
              class="settings-input"
              type="text"
              placeholder="-- bash -c {'{cmd}'}"
              bind:value={termCustomArgs}
            />
            <span class="settings-hint">Use {'{cmd}'} as placeholder for the resume command</span>
          </div>
        {/if}
        {#if settingsError}
          <div class="settings-error" role="alert">{settingsError}</div>
        {/if}
        <div class="settings-actions">
          <button class="modal-btn" onclick={() => (showSettings = false)}>Cancel</button>
          <button class="modal-btn modal-btn-primary" onclick={saveSettings}>
            {settingsSaved ? "Saved!" : "Save"}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

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

  .terminal-settings {
    width: 360px;
  }

  .settings-field {
    margin-bottom: 12px;
  }

  .settings-field:last-of-type {
    margin-bottom: 16px;
  }

  .settings-label {
    display: block;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary);
    margin-bottom: 6px;
  }

  .settings-radios {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .settings-radio {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-primary);
    cursor: pointer;
  }

  .settings-radio input {
    accent-color: var(--accent-blue);
  }

  .settings-hint {
    font-size: 10px;
    color: var(--text-muted);
    margin-left: auto;
  }

  .settings-input {
    width: 100%;
    padding: 5px 8px;
    font-size: 12px;
    font-family: var(--font-mono, "SF Mono", "Menlo", monospace);
    background: var(--bg-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
  }

  .settings-input:focus {
    border-color: var(--accent-blue);
    outline: none;
  }

  .settings-input::placeholder {
    color: var(--text-muted);
    opacity: 0.6;
  }

  .settings-field > .settings-hint {
    display: block;
    margin-left: 0;
    margin-top: 4px;
  }

  .settings-error {
    font-size: 11px;
    color: var(--accent-red, #f85149);
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid rgba(248, 81, 73, 0.25);
    border-radius: var(--radius-sm, 4px);
    padding: 6px 8px;
    margin-bottom: 12px;
  }

  .settings-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
