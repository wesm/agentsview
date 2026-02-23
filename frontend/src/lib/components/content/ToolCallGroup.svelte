<script lang="ts">
  import type { Message } from "../../api/types.js";
  import {
    parseContent,
    enrichSegments,
  } from "../../utils/content-parser.js";
  import { formatTimestamp } from "../../utils/format.js";
  import ToolBlock from "./ToolBlock.svelte";

  interface Props {
    messages: Message[];
    timestamp: string;
  }

  let { messages, timestamp }: Props = $props();

  let toolSegments = $derived(
    messages.flatMap((m) =>
      enrichSegments(
        parseContent(m.content, m.has_tool_use),
        m.tool_calls,
      ).filter((s) => s.type === "tool"),
    ),
  );

  let label = $derived(
    toolSegments.length === 1
      ? "1 tool call"
      : `${toolSegments.length} tool calls`,
  );
</script>

<div class="tool-group">
  <div class="tool-group-header">
    <span class="gear-icon">
      <svg width="12" height="12" viewBox="0 0 16 16"
        fill="var(--accent-amber)">
        <path d="M8 4.754a3.246 3.246 0 100 6.492
          3.246 3.246 0 000-6.492zM5.754 8a2.246
          2.246 0 114.492 0 2.246 2.246 0
          01-4.492 0z"/>
        <path d="M9.796 1.343c-.527-1.79-3.065-1.79-3.592
          0l-.094.319a.873.873 0
          01-1.255.52l-.292-.16c-1.64-.892-3.433.902-2.54
          2.541l.159.292a.873.873 0
          01-.52 1.255l-.319.094c-1.79.527-1.79 3.065
          0 3.592l.319.094a.873.873 0
          01.52 1.255l-.16.292c-.892 1.64.901 3.434
          2.541 2.54l.292-.159a.873.873 0
          011.255.52l.094.319c.527 1.79 3.065 1.79
          3.592 0l.094-.319a.873.873 0
          011.255-.52l.292.16c1.64.893 3.434-.902
          2.54-2.541l-.159-.292a.873.873 0
          01.52-1.255l.319-.094c1.79-.527 1.79-3.065
          0-3.592l-.319-.094a.873.873 0
          01-.52-1.255l.16-.292c.893-1.64-.902-3.433-2.541-2.54l-.292.159a.873.873
          0 01-1.255-.52l-.094-.319zm-2.633.283a.909.909
          0 011.674 0l.094.319a1.873 1.873 0
          002.693 1.115l.291-.16a.909.909 0
          011.18 1.18l-.159.292a1.873 1.873 0
          001.116 2.692l.318.094a.909.909 0
          010 1.674l-.319.094a1.873 1.873 0
          00-1.115 2.693l.16.291a.909.909 0
          01-1.18 1.18l-.292-.159a1.873 1.873 0
          00-2.692 1.116l-.094.318a.909.909 0
          01-1.674 0l-.094-.319a1.873 1.873 0
          00-2.693-1.115l-.291.16a.909.909 0
          01-1.18-1.18l.159-.292a1.873 1.873 0
          00-1.116-2.692l-.318-.094a.909.909 0
          010-1.674l.319-.094a1.873 1.873 0
          001.115-2.693l-.16-.291a.909.909 0
          011.18-1.18l.292.159a1.873 1.873 0
          002.692-1.116l.094-.318z"/>
      </svg>
    </span>
    <span class="group-label">{label}</span>
    <span class="group-timestamp">
      {formatTimestamp(timestamp)}
    </span>
  </div>

  <div class="tool-group-body">
    {#each toolSegments as segment}
      <ToolBlock
        content={segment.content}
        label={segment.label}
        toolCall={segment.toolCall}
      />
    {/each}
  </div>
</div>

<style>
  .tool-group {
    border-left: 3px solid var(--accent-amber);
    background: var(--tool-bg);
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
    padding: 8px 12px;
  }

  .tool-group-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }

  .gear-icon {
    display: flex;
    align-items: center;
    flex-shrink: 0;
  }

  .group-label {
    font-size: 12px;
    font-weight: 600;
    color: var(--accent-amber);
  }

  .group-timestamp {
    font-size: 12px;
    color: var(--text-muted);
    margin-left: auto;
  }

  .tool-group-body {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .tool-group-body :global(.tool-block) {
    margin: 0;
    border-left: none;
    border-radius: 0;
  }
</style>
