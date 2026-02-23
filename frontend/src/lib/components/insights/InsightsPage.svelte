<script lang="ts">
  import { onMount } from "svelte";
  import { insights } from "../../stores/insights.svelte.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { renderMarkdown } from "../../utils/markdown.js";
  import type { SummaryType, AgentName } from "../../api/types.js";

  function handleDateChange(e: Event) {
    const input = e.target as HTMLInputElement;
    insights.setDate(input.value);
  }

  function handleTypeChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    insights.setType(select.value as SummaryType);
  }

  function handleProjectChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    insights.setProject(select.value);
  }

  function handleAgentChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    insights.setAgent(select.value as AgentName);
  }

  function handleGenerate() {
    insights.generate();
  }

  function formatTime(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  function typeLabel(type: SummaryType): string {
    return type === "daily_activity"
      ? "Daily"
      : "Analysis";
  }

  onMount(() => {
    sessions.loadProjects();
    insights.load();
  });
</script>

<div class="insights-page">
  <div class="toolbar">
    <div class="toolbar-row">
      <input
        type="date"
        class="date-input"
        value={insights.date}
        onchange={handleDateChange}
      />

      <select
        class="type-select"
        value={insights.type}
        onchange={handleTypeChange}
      >
        <option value="daily_activity">Daily Activity</option>
        <option value="agent_analysis">Agent Analysis</option>
      </select>

      <select
        class="project-select"
        value={insights.project}
        onchange={handleProjectChange}
      >
        <option value="">All Projects (Global)</option>
        {#each sessions.projects as project}
          <option value={project.name}>
            {project.name}
          </option>
        {/each}
      </select>

      <select
        class="agent-select"
        value={insights.agent}
        onchange={handleAgentChange}
      >
        <option value="claude">Claude</option>
        <option value="codex">Codex</option>
        <option value="gemini">Gemini</option>
      </select>

      <button
        class="generate-btn"
        onclick={handleGenerate}
        disabled={insights.loading}
      >
        Generate
      </button>
    </div>

    <div class="toolbar-row">
      <textarea
        class="prompt-input"
        placeholder="Optional: add context or questions to steer the insight..."
        bind:value={insights.promptText}
        rows="2"
      ></textarea>
    </div>
  </div>

  <div class="body">
    <aside class="insight-list">
      {#if insights.tasks.length > 0}
        <div class="section-label">
          Generating
          {#if insights.tasks.length > 1}
            <button
              class="cancel-all-btn"
              onclick={() => insights.cancelAll()}
            >
              Cancel all
            </button>
          {/if}
        </div>
        {#each insights.tasks as task (task.clientId)}
          <div
            class="insight-item generating"
            class:error={task.status === "error"}
          >
            <div class="item-status">
              {#if task.status === "generating"}
                <span class="spinner-dot"></span>
              {:else}
                <span class="dot error-dot"></span>
              {/if}
            </div>
            <div class="item-info">
              <div class="item-name">
                {typeLabel(task.type)}
                {#if task.project}
                  - {task.project}
                {:else}
                  - global
                {/if}
              </div>
              <div class="item-meta">
                {#if task.status === "error"}
                  {task.error}
                {:else}
                  {task.phase}
                {/if}
              </div>
            </div>
            <button
              class="task-action-btn"
              onclick={() => task.status === "error"
                ? insights.dismissTask(task.clientId)
                : insights.cancelTask(task.clientId)}
              title={task.status === "error"
                ? "Dismiss"
                : "Cancel"}
            >
              {#if task.status === "error"}
                <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M4.646 4.646a.5.5 0 01.708 0L8 7.293l2.646-2.647a.5.5 0 01.708.708L8.707 8l2.647 2.646a.5.5 0 01-.708.708L8 8.707l-2.646 2.647a.5.5 0 01-.708-.708L7.293 8 4.646 5.354a.5.5 0 010-.708z"/>
                </svg>
              {:else}
                <svg width="10" height="10" viewBox="0 0 16 16" fill="currentColor">
                  <path d="M4.646 4.646a.5.5 0 01.708 0L8 7.293l2.646-2.647a.5.5 0 01.708.708L8.707 8l2.647 2.646a.5.5 0 01-.708.708L8 8.707l-2.646 2.647a.5.5 0 01-.708-.708L7.293 8 4.646 5.354a.5.5 0 010-.708z"/>
                </svg>
              {/if}
            </button>
          </div>
        {/each}
      {/if}

      {#if insights.loading}
        <div class="list-empty">Loading...</div>
      {:else if insights.summaries.length === 0 && insights.tasks.length === 0}
        <div class="list-empty">
          No insights yet. Click Generate to create one.
        </div>
      {:else}
        {#if insights.summaries.length > 0 && insights.tasks.length > 0}
          <div class="section-label">Completed</div>
        {/if}
        {#each insights.summaries as s (s.id)}
          <button
            class="insight-item completed"
            class:selected={insights.selectedId === s.id}
            onclick={() => insights.select(s.id)}
          >
            <div class="item-status">
              <span
                class="dot"
                class:dot-blue={s.type === "daily_activity"}
                class:dot-purple={s.type === "agent_analysis"}
              ></span>
            </div>
            <div class="item-info">
              <div class="item-name">
                {typeLabel(s.type)}
                {#if s.project}
                  - {s.project}
                {:else}
                  - global
                {/if}
              </div>
              <div class="item-meta">
                {formatTime(s.created_at)}
                {#if s.agent}
                  / {s.agent}
                {/if}
              </div>
            </div>
          </button>
        {/each}
      {/if}
    </aside>

    <main class="insight-content">
      {#if insights.selectedSummary}
        <div class="content-header">
          <span
            class="type-badge"
            class:badge-blue={insights.selectedSummary.type === "daily_activity"}
            class:badge-purple={insights.selectedSummary.type === "agent_analysis"}
          >
            {typeLabel(insights.selectedSummary.type)}
          </span>
          <span class="content-date">{insights.selectedSummary.date}</span>
          {#if insights.selectedSummary.project}
            <span class="content-project">{insights.selectedSummary.project}</span>
          {:else}
            <span class="content-project global">global</span>
          {/if}
          <span class="content-agent">
            {insights.selectedSummary.agent}
            {#if insights.selectedSummary.model}
              / {insights.selectedSummary.model}
            {/if}
          </span>
          <span class="content-time">
            {formatTime(insights.selectedSummary.created_at)}
          </span>
        </div>
        <div class="markdown-body">
          {@html renderMarkdown(insights.selectedSummary.content)}
        </div>
      {:else}
        <div class="content-empty">
          {#if insights.summaries.length > 0}
            Select an insight from the list
          {:else if insights.tasks.length > 0}
            Generating...
          {:else}
            Generate an insight to get started
          {/if}
        </div>
      {/if}
    </main>
  </div>
</div>

<style>
  .insights-page {
    display: flex;
    flex-direction: column;
    height: calc(100vh - 36px - 24px);
    overflow: hidden;
  }

  .toolbar {
    padding: 8px 12px;
    border-bottom: 1px solid var(--border-default);
    background: var(--bg-surface);
    display: flex;
    flex-direction: column;
    gap: 6px;
    flex-shrink: 0;
  }

  .toolbar-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .date-input,
  .type-select,
  .project-select,
  .agent-select {
    height: 28px;
    padding: 0 8px;
    background: var(--bg-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    font-size: 12px;
    color: var(--text-secondary);
  }

  .date-input:focus,
  .type-select:focus,
  .project-select:focus,
  .agent-select:focus {
    outline: none;
    border-color: var(--accent-blue);
  }

  .prompt-input {
    flex: 1;
    min-height: 28px;
    padding: 4px 8px;
    background: var(--bg-inset);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    font-size: 12px;
    color: var(--text-primary);
    font-family: var(--font-sans);
    resize: vertical;
  }

  .prompt-input:focus {
    outline: none;
    border-color: var(--accent-blue);
  }

  .prompt-input::placeholder {
    color: var(--text-muted);
  }

  .generate-btn {
    height: 28px;
    padding: 0 16px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    white-space: nowrap;
    background: var(--accent-blue);
    color: white;
  }

  .generate-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .generate-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .body {
    display: grid;
    grid-template-columns: 260px 1fr;
    flex: 1;
    overflow: hidden;
  }

  .insight-list {
    border-right: 1px solid var(--border-default);
    overflow-y: auto;
    background: var(--bg-surface);
  }

  .section-label {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 12px;
    font-size: 10px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    border-bottom: 1px solid var(--border-muted);
  }

  .cancel-all-btn {
    font-size: 10px;
    font-weight: 500;
    color: var(--text-muted);
    cursor: pointer;
    text-transform: none;
    letter-spacing: 0;
  }

  .cancel-all-btn:hover {
    color: var(--danger);
  }

  .list-empty {
    padding: 16px 12px;
    color: var(--text-muted);
    font-size: 12px;
    text-align: center;
  }

  .insight-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: 40px;
    padding: 0 12px;
    text-align: left;
    border-bottom: 1px solid var(--border-muted);
    transition: background 0.1s;
  }

  .insight-item.completed {
    cursor: pointer;
    border-left: 2px solid transparent;
  }

  .insight-item.completed:hover {
    background: var(--bg-surface-hover);
  }

  .insight-item.completed.selected {
    background: var(--bg-surface-hover);
    border-left-color: var(--accent-blue);
  }

  .insight-item.generating {
    animation: pulse 2s ease-in-out infinite;
  }

  .insight-item.generating.error {
    animation: none;
    background: var(--bg-inset);
  }

  .item-status {
    flex-shrink: 0;
    width: 10px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-muted);
  }

  .dot-blue {
    background: var(--accent-blue);
  }

  .dot-purple {
    background: var(--accent-purple);
  }

  .error-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--danger);
  }

  .spinner-dot {
    width: 8px;
    height: 8px;
    border: 1.5px solid var(--accent-blue);
    border-top-color: transparent;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .item-info {
    min-width: 0;
    flex: 1;
  }

  .item-name {
    font-size: 12px;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
  }

  .item-meta {
    font-size: 10px;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
  }

  .task-action-btn {
    flex-shrink: 0;
    width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    cursor: pointer;
  }

  .task-action-btn:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .insight-content {
    overflow-y: auto;
    padding: 16px 24px;
  }

  .content-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-bottom: 12px;
    margin-bottom: 12px;
    border-bottom: 1px solid var(--border-default);
    font-size: 12px;
    color: var(--text-secondary);
    flex-wrap: wrap;
  }

  .type-badge {
    font-size: 10px;
    font-weight: 600;
    padding: 2px 6px;
    border-radius: var(--radius-sm);
    color: white;
  }

  .badge-blue {
    background: var(--accent-blue);
  }

  .badge-purple {
    background: var(--accent-purple);
  }

  .content-project {
    font-size: 11px;
    padding: 1px 5px;
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    color: var(--text-secondary);
  }

  .content-project.global {
    font-style: italic;
    color: var(--text-muted);
  }

  .content-agent {
    color: var(--text-muted);
    font-size: 11px;
  }

  .content-date {
    font-weight: 500;
    color: var(--text-primary);
  }

  .content-time {
    color: var(--text-muted);
    font-size: 11px;
  }

  .content-empty {
    color: var(--text-muted);
    font-size: 13px;
    padding: 32px 0;
    text-align: center;
  }

  .markdown-body {
    font-size: 13px;
    line-height: 1.6;
    color: var(--text-primary);
  }

  .markdown-body :global(h1) {
    font-size: 18px;
    font-weight: 700;
    margin: 0 0 12px;
    padding-bottom: 6px;
    border-bottom: 1px solid var(--border-default);
  }

  .markdown-body :global(h2) {
    font-size: 15px;
    font-weight: 600;
    margin: 16px 0 8px;
  }

  .markdown-body :global(h3) {
    font-size: 13px;
    font-weight: 600;
    margin: 12px 0 6px;
  }

  .markdown-body :global(p) {
    margin: 0 0 8px;
  }

  .markdown-body :global(ul),
  .markdown-body :global(ol) {
    margin: 0 0 8px;
    padding-left: 20px;
  }

  .markdown-body :global(li) {
    margin: 2px 0;
  }

  .markdown-body :global(code) {
    font-family: var(--font-mono);
    font-size: 12px;
    padding: 1px 4px;
    background: var(--bg-inset);
    border-radius: var(--radius-sm);
  }

  .markdown-body :global(pre) {
    background: var(--bg-inset);
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    overflow-x: auto;
    margin: 0 0 8px;
  }

  .markdown-body :global(pre code) {
    padding: 0;
    background: transparent;
  }

  .markdown-body :global(blockquote) {
    margin: 0 0 8px;
    padding: 4px 12px;
    border-left: 3px solid var(--border-default);
    color: var(--text-secondary);
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.6; }
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>
