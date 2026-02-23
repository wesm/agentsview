<script lang="ts">
  import type { Message } from "../../api/types.js";
  import {
    parseContent,
    enrichSegments,
  } from "../../utils/content-parser.js";
  import { formatTimestamp } from "../../utils/format.js";
  import ThinkingBlock from "./ThinkingBlock.svelte";
  import ToolBlock from "./ToolBlock.svelte";
  import CodeBlock from "./CodeBlock.svelte";
  import { ui } from "../../stores/ui.svelte.js";
  import { renderMarkdown } from "../../utils/markdown.js";

  interface Props {
    message: Message;
  }

  let { message }: Props = $props();

  let segments = $derived(
    enrichSegments(
      parseContent(message.content, message.has_tool_use),
      message.tool_calls,
    ),
  );

  let isUser = $derived(message.role === "user");

  let accentColor = $derived(
    isUser ? "var(--accent-blue)" : "var(--accent-purple)",
  );

  let roleBg = $derived(
    isUser ? "var(--user-bg)" : "var(--assistant-bg)",
  );
</script>

<div
  class="message"
  class:is-user={isUser}
  style:border-left-color={accentColor}
  style:background={roleBg}
>
  <div class="message-header">
    <span
      class="role-icon"
      style:background={accentColor}
    >
      {isUser ? "U" : "A"}
    </span>
    <span
      class="role-label"
      style:color={accentColor}
    >
      {isUser ? "User" : "Assistant"}
    </span>
    <span class="timestamp">
      {formatTimestamp(message.timestamp)}
    </span>
  </div>

  <div class="message-body">
    {#each segments as segment}
      {#if segment.type === "thinking"}
        {#if ui.showThinking}
          <ThinkingBlock content={segment.content} />
        {/if}
      {:else if segment.type === "tool"}
        <ToolBlock
          content={segment.content}
          label={segment.label}
          toolCall={segment.toolCall}
        />
      {:else if segment.type === "code"}
        <CodeBlock content={segment.content} language={segment.label} />
      {:else}
        <div class="text-content markdown">
          {@html renderMarkdown(segment.content)}
        </div>
      {/if}
    {/each}
  </div>
</div>

<style>
  .message {
    border-left: 4px solid;
    padding: 14px 20px;
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
  }

  .message-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
  }

  .role-icon {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    font-weight: 700;
    color: white;
    flex-shrink: 0;
    line-height: 1;
  }

  .role-label {
    font-size: 13px;
    font-weight: 600;
    letter-spacing: 0.01em;
  }

  .timestamp {
    font-size: 12px;
    color: var(--text-muted);
    margin-left: auto;
  }

  .text-content {
    font-size: 14px;
    line-height: 1.7;
    color: var(--text-primary);
    word-wrap: break-word;
  }

  .message-body {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  /* Markdown prose styles */
  .markdown :global(p) {
    margin: 0.5em 0;
  }

  .markdown :global(p:first-child) {
    margin-top: 0;
  }

  .markdown :global(p:last-child) {
    margin-bottom: 0;
  }

  .markdown :global(h1),
  .markdown :global(h2),
  .markdown :global(h3),
  .markdown :global(h4),
  .markdown :global(h5),
  .markdown :global(h6) {
    margin: 0.8em 0 0.4em;
    line-height: 1.3;
    font-weight: 600;
  }

  .markdown :global(h1) { font-size: 1.35em; }
  .markdown :global(h2) { font-size: 1.2em; }
  .markdown :global(h3) { font-size: 1.1em; }
  .markdown :global(h4),
  .markdown :global(h5),
  .markdown :global(h6) { font-size: 1em; }

  .markdown :global(a) {
    color: var(--accent-blue);
    text-decoration: none;
  }

  .markdown :global(a:hover) {
    text-decoration: underline;
  }

  .markdown :global(code) {
    font-family: var(--font-mono);
    font-size: 0.85em;
    background: var(--bg-inset);
    border: 1px solid var(--border-muted);
    border-radius: 4px;
    padding: 0.15em 0.4em;
  }

  .markdown :global(pre) {
    background: var(--code-bg);
    color: var(--code-text);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    overflow-x: auto;
    margin: 0.5em 0;
  }

  .markdown :global(pre code) {
    background: none;
    border: none;
    padding: 0;
    font-size: 13px;
    color: inherit;
  }

  .markdown :global(blockquote) {
    border-left: 3px solid var(--border-default);
    margin: 0.5em 0;
    padding: 0.3em 1em;
    color: var(--text-secondary);
  }

  .markdown :global(ul),
  .markdown :global(ol) {
    padding-left: 1.6em;
    margin: 0.5em 0;
  }

  .markdown :global(li) {
    margin: 0.2em 0;
    line-height: 1.65;
  }

  .markdown :global(hr) {
    border: none;
    border-top: 1px solid var(--border-muted);
    margin: 0.8em 0;
  }

  .markdown :global(table) {
    border-collapse: collapse;
    margin: 0.5em 0;
    width: auto;
    font-size: 13px;
  }

  .markdown :global(th),
  .markdown :global(td) {
    border: 1px solid var(--border-muted);
    padding: 6px 10px;
    text-align: left;
  }

  .markdown :global(th) {
    background: var(--bg-inset);
    font-weight: 600;
  }

  .markdown :global(img) {
    max-width: 100%;
    border-radius: var(--radius-sm);
  }

  .markdown :global(strong) {
    font-weight: 600;
  }
</style>
