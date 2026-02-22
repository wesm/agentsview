<script lang="ts">
  interface Props {
    content: string;
    label?: string;
  }

  let { content, label }: Props = $props();
  let collapsed: boolean = $state(true);

  let previewLine = $derived(
    content.split("\n")[0]?.slice(0, 100) ?? "",
  );
</script>

<div class="tool-block">
  <button
    class="tool-header"
    onclick={() => (collapsed = !collapsed)}
  >
    <span class="tool-chevron" class:open={!collapsed}>
      &#9656;
    </span>
    {#if label}
      <span class="tool-label">{label}</span>
    {/if}
    {#if collapsed && previewLine}
      <span class="tool-preview">{previewLine}</span>
    {/if}
  </button>
  {#if !collapsed}
    <pre class="tool-content">{content}</pre>
  {/if}
</div>

<style>
  .tool-block {
    border-left: 2px solid var(--accent-amber);
    background: var(--tool-bg);
    border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
    margin: 0;
  }

  .tool-header {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 10px;
    width: 100%;
    text-align: left;
    font-size: 12px;
    color: var(--text-secondary);
    min-width: 0;
    border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
    transition: background 0.1s;
  }

  .tool-header:hover {
    background: var(--bg-surface-hover);
    color: var(--text-primary);
  }

  .tool-chevron {
    display: inline-block;
    font-size: 10px;
    transition: transform 0.15s;
    flex-shrink: 0;
    color: var(--text-muted);
  }

  .tool-chevron.open {
    transform: rotate(90deg);
  }

  .tool-label {
    font-family: var(--font-mono);
    font-weight: 500;
    font-size: 11px;
    color: var(--accent-amber);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .tool-preview {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .tool-content {
    padding: 8px 14px 10px;
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-secondary);
    line-height: 1.5;
    overflow-x: auto;
    border-top: 1px solid var(--border-muted);
  }
</style>
