import type {
  Summary,
  SummaryType,
  AgentName,
} from "../api/types.js";
import {
  listSummaries,
  generateSummary,
  type GenerateSummaryHandle,
} from "../api/client.js";

function localDateStr(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export interface InsightTask {
  clientId: string;
  type: SummaryType;
  date: string;
  project: string;
  agent: AgentName;
  status: "generating" | "done" | "error";
  phase: string;
  error: string | null;
  summaryId: number | null;
}

class InsightsStore {
  date: string = $state(localDateStr(new Date()));
  type: SummaryType = $state("daily_activity");
  project: string = $state("");
  agent: AgentName = $state("claude");
  summaries: Summary[] = $state([]);
  selectedId: number | null = $state(null);
  loading = $state(false);
  promptText: string = $state("");
  tasks: InsightTask[] = $state([]);

  #handles = new Map<string, GenerateSummaryHandle>();
  #version = 0;

  get selectedSummary(): Summary | undefined {
    return this.summaries.find(
      (s) => s.id === this.selectedId,
    );
  }

  get generatingCount(): number {
    return this.tasks.filter(
      (t) => t.status === "generating",
    ).length;
  }

  async load() {
    const v = ++this.#version;
    this.loading = true;
    try {
      const res = await listSummaries({
        type: this.type,
        date: this.date,
        project: this.project || undefined,
      });
      if (this.#version === v) {
        this.summaries = res.summaries;
        if (
          this.selectedId !== null &&
          !this.summaries.some(
            (s) => s.id === this.selectedId,
          )
        ) {
          this.selectedId = null;
        }
      }
    } catch {
      if (this.#version === v) {
        this.summaries = [];
      }
    } finally {
      if (this.#version === v) {
        this.loading = false;
      }
    }
  }

  setDate(date: string) {
    this.date = date;
    this.selectedId = null;
    this.load();
  }

  setType(type: SummaryType) {
    this.type = type;
    this.selectedId = null;
    this.load();
  }

  setProject(project: string) {
    this.project = project;
    this.selectedId = null;
    this.load();
  }

  setAgent(agent: AgentName) {
    this.agent = agent;
  }

  select(id: number) {
    this.selectedId = id;
  }

  generate() {
    const clientId = crypto.randomUUID();
    const snap = {
      type: this.type,
      date: this.date,
      project: this.project,
      agent: this.agent,
    };

    const task: InsightTask = {
      clientId,
      type: snap.type,
      date: snap.date,
      project: snap.project,
      agent: snap.agent,
      status: "generating",
      phase: "generating",
      error: null,
      summaryId: null,
    };
    this.tasks = [...this.tasks, task];

    const handle = generateSummary(
      {
        type: snap.type,
        date: snap.date,
        project: snap.project || undefined,
        prompt: this.promptText || undefined,
        agent: snap.agent,
      },
      (phase) => {
        this.tasks = this.tasks.map((t) =>
          t.clientId === clientId
            ? { ...t, phase }
            : t,
        );
      },
    );
    this.#handles.set(clientId, handle);

    handle.done
      .then((summary) => {
        this.#handles.delete(clientId);
        this.tasks = this.tasks.filter(
          (t) => t.clientId !== clientId,
        );

        const filtersMatch =
          this.type === snap.type &&
          this.date === snap.date &&
          this.project === snap.project;
        if (filtersMatch) {
          this.summaries = [summary, ...this.summaries];
          this.selectedId = summary.id;
        } else {
          this.load();
        }
      })
      .catch((e) => {
        this.#handles.delete(clientId);
        if (
          e instanceof DOMException &&
          e.name === "AbortError"
        ) {
          this.tasks = this.tasks.filter(
            (t) => t.clientId !== clientId,
          );
          return;
        }
        const msg =
          e instanceof Error
            ? e.message
            : "Generation failed";
        this.tasks = this.tasks.map((t) =>
          t.clientId === clientId
            ? { ...t, status: "error" as const, error: msg }
            : t,
        );
      });
  }

  cancelTask(clientId: string) {
    this.#handles.get(clientId)?.abort();
  }

  dismissTask(clientId: string) {
    this.#handles.delete(clientId);
    this.tasks = this.tasks.filter(
      (t) => t.clientId !== clientId,
    );
  }

  cancelAll() {
    for (const handle of this.#handles.values()) {
      handle.abort();
    }
  }
}

export const insights = new InsightsStore();
