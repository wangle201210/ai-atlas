import type { Snapshot } from "./types";
const backend = () => import("../bindings/ai-atlas/internal/atlas/service");

function downloadJSON(raw: string, filename: string): string {
  const url = URL.createObjectURL(
    new Blob([raw], { type: "application/json" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.append(link);
  try {
    link.click();
  } finally {
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 60000);
  }
  return "已发起下载，请在浏览器下载列表中查看。";
}
export async function exportReport(snapshot: Snapshot): Promise<string> {
  const b = await backend();
  if (!(await b.NativeFileDialogs()))
    return downloadJSON(
      JSON.stringify(snapshot, null, 2),
      `ai-atlas-${new Date().toISOString().slice(0, 10)}.json`,
    );
  const path = await b.ExportReport(snapshot);
  return path ? `报表已保存至：${path}` : "";
}
export async function exportDiagnostics(): Promise<string> {
  const b = await backend();
  if (!(await b.NativeFileDialogs()))
    return downloadJSON(await b.Diagnostics(), "ai-atlas-diagnostics.json");
  const path = await b.ExportDiagnostics();
  return path ? `诊断已保存至：${path}` : "";
}
