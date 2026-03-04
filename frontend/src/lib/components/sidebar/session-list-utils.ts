import type { SessionGroup } from "../../stores/sessions.svelte.js";

export const ITEM_HEIGHT = 42;
export const HEADER_HEIGHT = 28;
export const OVERSCAN = 10;
export const STORAGE_KEY = "agentsview-group-by-agent";

export interface AgentSection {
  agent: string;
  groups: SessionGroup[];
}

export interface DisplayItem {
  id: string;
  type: "header" | "session";
  agent: string;
  count: number;
  group?: SessionGroup;
  height: number;
  top: number;
}

/**
 * Build agent-grouped sections from flat session groups.
 * Returns empty array when grouping is off.
 */
export function buildAgentSections(
  groups: SessionGroup[],
  groupByAgent: boolean,
): AgentSection[] {
  if (!groupByAgent) return [];
  const map = new Map<string, SessionGroup[]>();
  for (const g of groups) {
    const primary =
      g.sessions.find((s) => s.id === g.primarySessionId) ??
      g.sessions[0];
    if (!primary) continue;
    const agent = primary.agent;
    let list = map.get(agent);
    if (!list) {
      list = [];
      map.set(agent, list);
    }
    list.push(g);
  }
  // Sort by count descending (most sessions first).
  return Array.from(map.entries())
    .sort((a, b) => b[1].length - a[1].length)
    .map(([agent, groups]) => ({ agent, groups }));
}

/**
 * Build a flat list of DisplayItems for virtual scrolling.
 * When `groupByAgent` is false, produces a simple flat list.
 * When true, interleaves header rows and session rows,
 * respecting collapsed agents.
 */
export function buildDisplayItems(
  groups: SessionGroup[],
  agentSections: AgentSection[],
  groupByAgent: boolean,
  collapsedAgents: Set<string>,
): DisplayItem[] {
  if (!groupByAgent) {
    return groups.map((g, i) => ({
      id: `session:${g.primarySessionId}`,
      type: "session" as const,
      agent: "",
      count: 0,
      group: g,
      height: ITEM_HEIGHT,
      top: i * ITEM_HEIGHT,
    }));
  }

  const items: DisplayItem[] = [];
  let y = 0;
  for (const section of agentSections) {
    items.push({
      id: `header:${section.agent}`,
      type: "header",
      agent: section.agent,
      count: section.groups.length,
      height: HEADER_HEIGHT,
      top: y,
    });
    y += HEADER_HEIGHT;

    if (!collapsedAgents.has(section.agent)) {
      for (const g of section.groups) {
        items.push({
          id: `session:${section.agent}:${g.primarySessionId}`,
          type: "session",
          agent: section.agent,
          count: 0,
          group: g,
          height: ITEM_HEIGHT,
          top: y,
        });
        y += ITEM_HEIGHT;
      }
    }
  }
  return items;
}

/**
 * Compute total pixel height of the display items list.
 */
export function computeTotalSize(displayItems: DisplayItem[]): number {
  if (displayItems.length === 0) return 0;
  const last = displayItems[displayItems.length - 1]!;
  return last.top + last.height;
}

/**
 * Binary search for the index of the first visible item given
 * scrollY position.  Accounts for OVERSCAN rows before the
 * viewport.
 */
export function findStart(
  displayItems: DisplayItem[],
  scrollY: number,
): number {
  const target = scrollY - OVERSCAN * ITEM_HEIGHT;
  let lo = 0;
  let hi = displayItems.length - 1;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (displayItems[mid]!.top + displayItems[mid]!.height <= target) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  return Math.max(0, lo);
}
