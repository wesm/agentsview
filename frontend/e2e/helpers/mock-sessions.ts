import type { Route } from "@playwright/test";

interface MockSession {
  id: string;
  project: string;
  machine: string;
  agent: string;
  first_message: string;
  started_at: string;
  ended_at: string;
  message_count: number;
  created_at: string;
  file_path: string;
}

export function createMockSessions(
  count: number,
  prefix: string,
  projectFn: (index: number) => string,
): MockSession[] {
  const now = new Date().toISOString();
  return Array.from({ length: count }, (_, i) => ({
    id: `${prefix}-${i}`,
    project: projectFn(i),
    machine: "test-machine",
    agent: "test-agent",
    first_message: `Hello from ${prefix} ${i}`,
    started_at: now,
    ended_at: now,
    message_count: 10,
    created_at: now,
    file_path: `/tmp/${prefix}-${i}.json`,
  }));
}

interface SessionDataSet {
  sessions: MockSession[];
  project: string | null;
}

/**
 * Creates a route handler that serves paginated, filterable
 * session data from the provided data sets.
 */
export function handleSessionsRoute(
  dataSets: SessionDataSet[],
) {
  return async (route: Route) => {
    const url = new URL(route.request().url());
    const limit = Number(url.searchParams.get("limit") || "200");
    const cursor = url.searchParams.get("cursor");
    const project = url.searchParams.get("project");

    const defaultSet = dataSets.find((d) => d.project === null);
    const matchedSet = project
      ? dataSets.find((d) => d.project === project)
      : null;

    let filtered: MockSession[];
    if (matchedSet) {
      filtered = matchedSet.sessions;
    } else if (defaultSet) {
      filtered = project
        ? defaultSet.sessions.filter((s) => s.project === project)
        : defaultSet.sessions;
    } else {
      filtered = [];
    }

    const startIndex = cursor ? parseInt(cursor, 10) : 0;
    const slice = filtered.slice(startIndex, startIndex + limit);
    const nextCursor =
      startIndex + limit < filtered.length
        ? (startIndex + limit).toString()
        : undefined;

    await route.fulfill({
      json: {
        sessions: slice,
        next_cursor: nextCursor,
        total: filtered.length,
      },
    });
  };
}
