import type {
  Snapshot,
  ScanStatus,
  SessionQuery,
  SessionPage,
  Detail,
  TempFile,
  CleanPlan,
  CleanResult,
} from "./types";
// Normalize nullable Go slices at the transport boundary.
const backend = () => import("../bindings/ai-atlas/internal/atlas/service");
export const api = {
  overview: async (since = "", until = ""): Promise<Snapshot> => {
    const v = await (await backend()).Overview(since, until);
    return {
      ...v,
      projects: v.projects ?? [],
      days: v.days ?? [],
      storage: v.storage ?? [],
      tempRoots: v.tempRoots ?? [],
    };
  },
  status: async (): Promise<ScanStatus> => {
    const v = await (await backend()).Status();
    return { ...v, errors: v.errors ?? [], failedFiles: v.failedFiles ?? [] };
  },
  cancelScan: async () => (await backend()).CancelScan(),
  retryFailed: async () => (await backend()).RetryFailed(),
  confirmTemp: async (path: string, project: string) =>
    (await backend()).ConfirmTempProject(path, project),
  messages: async (
    id: string,
    query: string,
    page: number,
  ): Promise<Detail> => {
    const v = await (await backend()).SessionMessages(id, query, page);
    return { ...v, messages: v.messages ?? [] };
  },
  scan: async (temps: boolean) => (await backend()).StartScan(temps),
  sessions: async (query: SessionQuery): Promise<SessionPage> => {
    const v = await (await backend()).Sessions(query);
    return { ...v, items: v.items ?? [] };
  },
  detail: async (id: string): Promise<Detail> => {
    const v = await (await backend()).SessionDetail(id);
    return { ...v, messages: v.messages ?? [] };
  },
  temps: async (): Promise<TempFile[]> =>
    ((await (await backend()).TempFiles()) ?? []).map((v) => ({
      ...v,
      evidence: v.evidence ?? [],
    })),
  plan: async (kind: string, ids: string[]): Promise<CleanPlan> => {
    const v = await (await backend()).PreviewClean(kind, ids);
    return { ...v, items: v.items ?? [] };
  },
  clean: async (token: string, text: string): Promise<CleanResult[]> =>
    (await (await backend()).ExecuteClean(token, text)) ?? [],
  archive: async (id: string, value: boolean) =>
    (await backend()).SetArchived(id, value),
  resume: async (id: string): Promise<string> =>
    (await backend()).ResumeCommand(id),
};
