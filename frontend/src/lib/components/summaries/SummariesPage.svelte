<script lang="ts">
  import { onMount } from "svelte";
  import { summaries } from "../../stores/summaries.svelte.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { renderMarkdown } from "../../utils/markdown.js";
  import type { SummaryType, AgentName } from "../../api/types.js";

  function handleDateChange(e: Event) {
    const input = e.target as HTMLInputElement;
    summaries.setDate(input.value);
  }

  function handleTypeChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    summaries.setType(select.value as SummaryType);
  }

  function handleProjectChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    summaries.setProject(select.value);
  }

  function handleAgentChange(e: Event) {
    const select = e.target as HTMLSelectElement;
    summaries.setAgent(select.value as AgentName);
  }

  function handleGenerate() {
    summaries.generate();
  }

  function handleCancel() {
    summaries.cancelGeneration();
  }

  function formatTime(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  onMount(() => {
    sessions.loadProjects();
    summaries.load();
  });
</script>

<div class="summaries-page">
  <div class="toolbar">
    <div class="toolbar-row">
      <input
        type="date"
        class="date-input"
        value={summaries.date}
        onchange={handleDateChange}
      />

      <select
        class="type-select"
        value={summaries.type}
        onchange={handleTypeChange}
      >
        <option value="daily_activity">Daily Activity</option>
        <option value="agent_analysis">Agent Analysis</option>
      </select>

      <select
        class="project-select"
        value={summaries.project}
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
        value={summaries.agent}
        onchange={handleAgentChange}
      >
        <option value="claude">Claude</option>
        <option value="codex">Codex</option>
        <option value="gemini">Gemini</option>
      </select>

      <button
        class="generate-btn"
        onclick={summaries.generating ? handleCancel : handleGenerate}
        disabled={summaries.loading}
      >
        {#if summaries.generating}
          Cancel
        {:else}
          Generate
        {/if}
      </button>
    </div>

    <div class="toolbar-row">
      <textarea
        class="prompt-input"
        placeholder="Optional: add context or questions to steer the summary..."
        bind:value={summaries.promptText}
        rows="2"
      ></textarea>
    </div>
  </div>

  <div class="body">
    <aside class="summary-list">
      {#if summaries.loading}
        <div class="list-empty">Loading...</div>
      {:else if summaries.summaries.length === 0}
        <div class="list-empty">
          No summaries yet.
          {#if !summaries.generating}
            Click Generate to create one.
          {/if}
        </div>
      {:else}
        {#each summaries.summaries as s (s.id)}
          <button
            class="summary-item"
            class:selected={summaries.selectedId === s.id}
            onclick={() => summaries.select(s.id)}
          >
            <span class="item-time">
              {formatTime(s.created_at)}
            </span>
            <span class="item-meta">
              {s.agent}
              {#if s.model}
                / {s.model}
              {/if}
            </span>
            {#if s.project}
              <span class="item-project">{s.project}</span>
            {:else}
              <span class="item-project global">global</span>
            {/if}
          </button>
        {/each}
      {/if}
    </aside>

    <main class="summary-content">
      {#if summaries.generating}
        <div class="content-status">
          Generating summary...
          {#if summaries.generatePhase}
            ({summaries.generatePhase})
          {/if}
        </div>
      {:else if summaries.generateError}
        <div class="content-error">
          {summaries.generateError}
        </div>
      {:else if summaries.selectedSummary}
        <div class="markdown-body">
          {@html renderMarkdown(summaries.selectedSummary.content)}
        </div>
      {:else}
        <div class="content-empty">
          {#if summaries.summaries.length > 0}
            Select a summary from the list
          {:else}
            Generate a summary to get started
          {/if}
        </div>
      {/if}
    </main>
  </div>
</div>

<style>
  .summaries-page {
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
    grid-template-columns: 240px 1fr;
    flex: 1;
    overflow: hidden;
  }

  .summary-list {
    border-right: 1px solid var(--border-default);
    overflow-y: auto;
    background: var(--bg-surface);
  }

  .list-empty {
    padding: 16px 12px;
    color: var(--text-muted);
    font-size: 12px;
    text-align: center;
  }

  .summary-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    padding: 8px 12px;
    text-align: left;
    border-bottom: 1px solid var(--border-default);
    cursor: pointer;
    background: transparent;
    transition: background 0.1s;
  }

  .summary-item:hover {
    background: var(--bg-surface-hover);
  }

  .summary-item.selected {
    background: var(--bg-surface-hover);
    border-left: 2px solid var(--accent-blue);
    padding-left: 10px;
  }

  .item-time {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .item-meta {
    font-size: 10px;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .item-project {
    display: inline-block;
    font-size: 10px;
    padding: 0 4px;
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    color: var(--text-secondary);
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .item-project.global {
    font-style: italic;
    color: var(--text-muted);
  }

  .summary-content {
    overflow-y: auto;
    padding: 16px 24px;
  }

  .content-status {
    color: var(--text-muted);
    font-size: 13px;
    padding: 32px 0;
    text-align: center;
  }

  .content-error {
    color: var(--danger);
    font-size: 13px;
    padding: 16px;
    background: var(--bg-inset);
    border-radius: var(--radius-sm);
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
</style>
