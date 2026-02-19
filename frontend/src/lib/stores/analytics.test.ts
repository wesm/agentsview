import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
} from "vitest";
import { analytics } from "./analytics.svelte.js";
import * as api from "../api/client.js";
import type {
  AnalyticsSummary,
  ActivityResponse,
  HeatmapResponse,
  ProjectsAnalyticsResponse,
} from "../api/types.js";

vi.mock("../api/client.js", () => ({
  getAnalyticsSummary: vi.fn(),
  getAnalyticsActivity: vi.fn(),
  getAnalyticsHeatmap: vi.fn(),
  getAnalyticsProjects: vi.fn(),
}));

vi.mock("./router.svelte.js", () => ({
  router: { navigate: vi.fn() },
}));

function makeSummary(): AnalyticsSummary {
  return {
    total_sessions: 10,
    total_messages: 100,
    active_projects: 3,
    active_days: 5,
    avg_messages: 10,
    median_messages: 8,
    p90_messages: 20,
    most_active_project: "proj",
    concentration: 0.5,
    agents: {},
  };
}

function makeActivity(): ActivityResponse {
  return { granularity: "day", series: [] };
}

function makeHeatmap(): HeatmapResponse {
  return {
    metric: "messages",
    entries: [],
    levels: { l1: 1, l2: 5, l3: 10, l4: 20 },
  };
}

function makeProjects(): ProjectsAnalyticsResponse {
  return { projects: [] };
}

function mockAllAPIs() {
  vi.mocked(api.getAnalyticsSummary).mockResolvedValue(
    makeSummary(),
  );
  vi.mocked(api.getAnalyticsActivity).mockResolvedValue(
    makeActivity(),
  );
  vi.mocked(api.getAnalyticsHeatmap).mockResolvedValue(
    makeHeatmap(),
  );
  vi.mocked(api.getAnalyticsProjects).mockResolvedValue(
    makeProjects(),
  );
}

function resetStore() {
  analytics.selectedDate = null;
  analytics.from = "2024-01-01";
  analytics.to = "2024-01-31";
}

// Assert the most recent call to a mocked API function used
// the expected from/to params. Uses lastCall so it reads the
// right invocation even if the mock was called multiple times.
function assertParams(
  fn: ReturnType<typeof vi.fn>,
  from: string,
  to: string,
) {
  const mock = vi.mocked(fn);
  expect(mock).toHaveBeenCalled();
  const params = mock.mock.lastCall![0];
  expect(params.from).toBe(from);
  expect(params.to).toBe(to);
}

// Note: selectDate and setDateRange invoke API mocks
// synchronously (the mock call is recorded before the first
// await inside fetchSummary/etc.), so no async flushing is
// needed for call-count or call-param assertions.

describe("AnalyticsStore.selectDate", () => {
  beforeEach(() => {
    resetStore();
    vi.clearAllMocks();
    mockAllAPIs();
  });

  it("should set selectedDate on first click", () => {
    analytics.selectDate("2024-01-15");
    expect(analytics.selectedDate).toBe("2024-01-15");
  });

  it("should deselect when clicking the same date", () => {
    analytics.selectDate("2024-01-15");
    analytics.selectDate("2024-01-15");
    expect(analytics.selectedDate).toBeNull();
  });

  it("should switch to a different date", () => {
    analytics.selectDate("2024-01-15");
    analytics.selectDate("2024-01-20");
    expect(analytics.selectedDate).toBe("2024-01-20");
  });

  it("should fetch summary, activity, projects but not heatmap", () => {
    analytics.selectDate("2024-01-15");

    expect(api.getAnalyticsSummary).toHaveBeenCalledTimes(1);
    expect(api.getAnalyticsActivity).toHaveBeenCalledTimes(1);
    expect(api.getAnalyticsProjects).toHaveBeenCalledTimes(1);
    expect(api.getAnalyticsHeatmap).not.toHaveBeenCalled();
  });

  it("should pass selected date as from/to for filtered panels", () => {
    analytics.selectDate("2024-01-15");

    assertParams(
      api.getAnalyticsSummary, "2024-01-15", "2024-01-15",
    );
    assertParams(
      api.getAnalyticsActivity, "2024-01-15", "2024-01-15",
    );
    assertParams(
      api.getAnalyticsProjects, "2024-01-15", "2024-01-15",
    );
  });

  it("should use full range after deselecting", () => {
    analytics.selectDate("2024-01-15");
    vi.clearAllMocks();
    mockAllAPIs();

    analytics.selectDate("2024-01-15"); // deselect

    assertParams(
      api.getAnalyticsSummary, "2024-01-01", "2024-01-31",
    );
    assertParams(
      api.getAnalyticsActivity, "2024-01-01", "2024-01-31",
    );
    assertParams(
      api.getAnalyticsProjects, "2024-01-01", "2024-01-31",
    );
  });
});

describe("AnalyticsStore.setDateRange", () => {
  beforeEach(() => {
    resetStore();
    vi.clearAllMocks();
    mockAllAPIs();
  });

  it("should clear selectedDate", () => {
    analytics.selectDate("2024-01-15");
    analytics.setDateRange("2024-02-01", "2024-02-28");
    expect(analytics.selectedDate).toBeNull();
  });

  it("should fetch all panels with new range params", () => {
    analytics.setDateRange("2024-02-01", "2024-02-28");

    expect(api.getAnalyticsSummary).toHaveBeenCalledTimes(1);
    expect(api.getAnalyticsActivity).toHaveBeenCalledTimes(1);
    expect(api.getAnalyticsHeatmap).toHaveBeenCalledTimes(1);
    expect(api.getAnalyticsProjects).toHaveBeenCalledTimes(1);

    assertParams(
      api.getAnalyticsSummary, "2024-02-01", "2024-02-28",
    );
    assertParams(
      api.getAnalyticsActivity, "2024-02-01", "2024-02-28",
    );
    assertParams(
      api.getAnalyticsHeatmap, "2024-02-01", "2024-02-28",
    );
    assertParams(
      api.getAnalyticsProjects, "2024-02-01", "2024-02-28",
    );
  });
});

describe("AnalyticsStore heatmap uses full range", () => {
  beforeEach(() => {
    resetStore();
    vi.clearAllMocks();
    mockAllAPIs();
  });

  it("should use base from/to for heatmap even with selectedDate", async () => {
    analytics.selectDate("2024-01-15");
    vi.clearAllMocks();
    mockAllAPIs();

    await analytics.fetchHeatmap();

    assertParams(
      api.getAnalyticsHeatmap, "2024-01-01", "2024-01-31",
    );
  });
});
