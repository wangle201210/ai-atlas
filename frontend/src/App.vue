<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import {
  Activity,
  ArrowDownToLine,
  ArrowRight,
  Archive,
  Check,
  ChevronLeft,
  ChevronRight,
  Database,
  FileText,
  Folder,
  FolderSearch,
  HardDrive,
  History,
  LayoutDashboard,
  RefreshCw,
  Search,
  Settings2,
  ShieldCheck,
  Terminal,
  Trash2,
  X,
} from "lucide-vue-next";
import { Clipboard } from "@wailsio/runtime";
import { exportReport } from "./file-export";
import { api } from "./api";
import { rememberedChoice } from "./preferences";
import appIcon from "./assets/token-signal.svg";
import UpdateControl from "./components/UpdateControl.vue";
import PreferencesPanel from "./components/PreferencesPanel.vue";
import RecoveryPanel from "./components/RecoveryPanel.vue";
import type {
  Snapshot,
  ScanStatus,
  Session,
  SessionPage,
  Detail,
  TempFile,
  CleanPlan,
  CleanResult,
} from "./types";
import "./style.css";
const tabs = [
  { id: "overview", label: "项目概览", icon: LayoutDashboard },
  { id: "sessions", label: "会话管理", icon: FileText },
  { id: "storage", label: "存储空间", icon: HardDrive },
  { id: "temps", label: "临时文件", icon: FolderSearch },
  { id: "history", label: "清理历史", icon: History },
];
const tab = ref("overview"),
  data = ref<Snapshot>(),
  status = ref<ScanStatus>({
    running: false,
    phase: "",
    done: 0,
    total: 0,
    errors: [],
    finished: "",
  });
const initialLoading = ref(true),
  initialized = ref(false);
const busy = ref(false),
  error = ref(""),
  notice = ref(""),
  search = ref(""),
  project = ref(""),
  state = ref("all"),
  sort = rememberedChoice(
    "session-sort",
    ["updated", "size", "tokens"],
    "updated",
  ),
  page = ref(0),
  since = ref(""),
  until = ref("");
const sessions = ref<SessionPage>({ items: [], total: 0 }),
  temps = ref<TempFile[]>([]),
  tempFilter = ref("linked"),
  selected = ref<string[]>([]),
  detail = ref<Detail>(),
  evidence = ref<TempFile>(),
  plan = ref<CleanPlan>(),
  confirmation = ref(""),
  results = ref<CleanResult[]>([]),
  settings = ref(false);
const dialog = ref<HTMLDialogElement>(),
  projectSort = rememberedChoice("project-sort", ["tokens", "bytes"], "tokens"),
  resume = ref(""),
  copyNotice = ref(""),
  messageSearch = ref(""),
  messagePage = ref(0),
  confirmedProject = ref("");
const title = computed(
  () => tabs.find((t) => t.id === tab.value)?.label || "项目概览",
);
const projects = computed(() =>
  [...(data.value?.projects || [])]
    .filter((p) =>
      (p.path + p.name).toLowerCase().includes(search.value.toLowerCase()),
    )
    .sort((a, b) =>
      projectSort.value === "bytes"
        ? b.bytes - a.bytes
        : b.usage.total - a.usage.total,
    ),
);
const filteredTemps = computed(() =>
  temps.value.filter(
    (t) =>
      (t.path + t.evidence.map((e) => e.project).join(" "))
        .toLowerCase()
        .includes(search.value.toLowerCase()) &&
      (tempFilter.value === "all" ||
        (tempFilter.value === "linked"
          ? t.evidence.length > 0
          : t.evidence.length === 0)),
  ),
);
const visibleTemps = computed(() =>
  filteredTemps.value.slice(page.value * 50, (page.value + 1) * 50),
);
const totalPages = computed(() =>
  Math.max(
    1,
    Math.ceil(
      (tab.value === "sessions"
        ? sessions.value.total
        : filteredTemps.value.length) / 50,
    ),
  ),
);
const maxDay = computed(() =>
  Math.max(1, ...(data.value?.days || []).map((d) => d.total)),
);
const storageTotal = computed(() =>
  (data.value?.storage || []).reduce((a, b) => a + b.bytes, 0),
);
const cacheRatio = computed(() =>
  data.value?.usage.input
    ? Math.round((data.value.usage.cached / data.value.usage.input) * 100)
    : 0,
);
const modalOpen = computed(
  () => !!detail.value || !!evidence.value || !!plan.value || settings.value,
);
const count = (n = 0) =>
  Intl.NumberFormat("zh-CN", {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(n);
const number = (n = 0) => Intl.NumberFormat("zh-CN").format(n);
const bytes = (n = 0) => {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let i = 0;
  while (n >= 1024 && i < 4) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
};
const date = (s: string) =>
  s
    ? new Date(s).toLocaleString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";
const basename = (p: string) => p.split("/").filter(Boolean).pop() || p;
let poll: ReturnType<typeof setInterval> | undefined,
  debounce: ReturnType<typeof setTimeout> | undefined,
  noticeTimer: ReturnType<typeof setTimeout> | undefined,
  copyNoticeTimer: ReturnType<typeof setTimeout> | undefined,
  polling = false,
  requestID = 0;
function clearNotice() {
  clearTimeout(noticeTimer);
  notice.value = "";
}
function showNotice(message: string) {
  clearNotice();
  notice.value = message;
  noticeTimer = setTimeout(clearNotice, 10_000);
}
function clearCopyNotice() {
  clearTimeout(copyNoticeTimer);
  copyNotice.value = "";
}
function showCopyNotice(message: string) {
  clearCopyNotice();
  copyNotice.value = message;
  copyNoticeTimer = setTimeout(clearCopyNotice, 10_000);
}
async function perform(fn: () => Promise<void>) {
  busy.value = true;
  error.value = "";
  try {
    await fn();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
async function loadOverview() {
  data.value = await api.overview(since.value, until.value);
}
async function loadSessions() {
  const id = ++requestID;
  const res = await api.sessions({
    search: search.value,
    project: project.value,
    state: state.value,
    sort: sort.value,
    page: page.value,
  });
  if (id === requestID) sessions.value = res;
}
async function refresh() {
  await loadOverview();
  if (tab.value === "sessions") await loadSessions();
  temps.value = await api.temps();
}
async function scan(withTemps = false) {
  await perform(async () => {
    await api.scan(withTemps);
    status.value = await api.status();
    showNotice("扫描在后台进行，可以继续浏览已有数据。");
  });
}
async function cancelScan() {
  await perform(async () => {
    await api.cancelScan();
  });
}
async function retryScan() {
  await perform(async () => {
    await api.retryFailed();
    status.value = await api.status();
  });
}
async function tick() {
  if (polling || !initialized.value) return;
  polling = true;
  try {
    const old = status.value.running;
    status.value = await api.status();
    if (old && !status.value.running) {
      await refresh();
      showNotice("索引已更新");
    }
  } catch (e) {
    error.value = String(e);
  } finally {
    polling = false;
  }
}
function navigate(id: string) {
  clearNotice();
  tab.value = id;
  search.value = "";
  page.value = 0;
  selected.value = [];
}
function showProject(path: string) {
  navigate("sessions");
  project.value = path;
  void perform(loadSessions);
}
function toggle(id: string) {
  selected.value = selected.value.includes(id)
    ? selected.value.filter((v) => v !== id)
    : [...selected.value, id];
}
function togglePage() {
  const ids =
    tab.value === "sessions"
      ? sessions.value.items.filter((v) => !v.missing).map((v) => v.id)
      : visibleTemps.value.map((v) => v.path);
  const all = ids.every((id) => selected.value.includes(id));
  selected.value = all
    ? selected.value.filter((v) => !ids.includes(v))
    : [...new Set([...selected.value, ...ids])].slice(0, 50);
}
async function openDetail(v: Session) {
  clearCopyNotice();
  await perform(async () => {
    messageSearch.value = "";
    messagePage.value = 0;
    detail.value = await api.detail(v.id);
    resume.value = "";
  });
}
async function loadMessages(page: number) {
  if (!detail.value) return;
  await perform(async () => {
    detail.value = await api.messages(
      detail.value!.session.id,
      messageSearch.value,
      page,
    );
    messagePage.value = page;
  });
}
async function confirmTemp() {
  if (!evidence.value || !confirmedProject.value) return;
  await perform(async () => {
    await api.confirmTemp(evidence.value!.path, confirmedProject.value);
    temps.value = await api.temps();
    evidence.value = temps.value.find((v) => v.path === evidence.value!.path);
    showNotice("归属确认已记录，仅对本次扫描有效");
  });
}
async function preview() {
  await perform(async () => {
    plan.value = await api.plan(
      tab.value === "sessions" ? "session" : "temp",
      selected.value,
    );
    confirmation.value = "";
    results.value = [];
  });
}
async function setForceCleanup(event: Event) {
  if (!plan.value || busy.value) return;
  const force = (event.target as HTMLInputElement).checked;
  const ids = plan.value.items.map(item => item.id);
  confirmation.value = "";
  await perform(async () => {
    plan.value = force ? await api.forceTempPlan(ids) : await api.plan("temp", ids);
  });
  (event.target as HTMLInputElement).checked = !!plan.value?.force;
}
async function execute() {
  if (!plan.value) return;
  await perform(async () => {
    results.value = await api.clean(plan.value!.token, confirmation.value);
    selected.value = [];
    await refresh();
    showNotice("清理结果已记录");
  });
}
async function archive(v: Session) {
  await perform(async () => {
    await api.archive(v.id, !v.archived);
    await refresh();
    detail.value = undefined;
    showNotice(v.archived ? "会话已恢复" : "会话已归档（不会释放日志空间）");
  });
}
async function copyResume(v: Session) {
  await perform(async () => {
    resume.value = await api.resume(v.id);
    try {
      let nativeCopied = false;
      try {
        await Clipboard.SetText(resume.value);
        // SetText discards the platform result; server mode is a silent no-op.
        nativeCopied = (await Clipboard.Text()) === resume.value;
      } catch {
        // Browser-only builds may not provide a native clipboard bridge.
      }
      if (!nativeCopied) await navigator.clipboard.writeText(resume.value);
      showCopyNotice("命令已复制到剪贴板，可直接粘贴到终端。");
    } catch {
      showCopyNotice("自动复制失败，请手动复制下方命令。");
    }
  });
}
function closeModal() {
  if (busy.value) return;
  clearCopyNotice();
  detail.value = undefined;
  evidence.value = undefined;
  plan.value = undefined;
  settings.value = false;
  results.value = [];
}
async function downloadReport() {
  if (!data.value || busy.value) return;
  const snapshot = data.value;
  clearNotice();
  await perform(async () => {
    const message = await exportReport(snapshot);
    if (message) showNotice(message);
  });
}
watch(modalOpen, async (open) => {
  await nextTick();
  if (open && !dialog.value?.open) dialog.value?.showModal();
  else if (!open && dialog.value?.open) dialog.value.close();
});
watch(tab, () => {
  if (tab.value === "sessions") void perform(loadSessions);
});
watch([search, project, state, sort], () => {
  page.value = 0;
  selected.value = [];
  clearTimeout(debounce);
  if (tab.value === "sessions")
    debounce = setTimeout(() => void perform(loadSessions), 250);
});
watch(page, () => {
  selected.value = [];
  if (tab.value === "sessions") void perform(loadSessions);
});
watch(tempFilter, () => {
  page.value = 0;
  selected.value = [];
});
watch([since, until], () => void perform(loadOverview));
async function initialize() {
  initialLoading.value = true;
  await perform(async () => {
    await refresh();
    status.value = await api.status();
    initialized.value = true;
  });
  initialLoading.value = false;
}
onMounted(() => {
  void initialize();
  poll = setInterval(tick, 1200);
});
onUnmounted(() => {
  clearInterval(poll);
  clearTimeout(debounce);
  clearNotice();
  clearCopyNotice();
});
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <a class="brand" href="#" @click.prevent="navigate('overview')"
        ><span class="brand-mark"><img :src="appIcon" alt="" /></span
        ><span
          >AI <strong>Atlas</strong><small>LOCAL WORKSPACE MANAGER</small></span
        ></a
      >
      <div class="nav-caption">工作空间</div>
      <nav aria-label="主导航">
        <button
          v-for="item in tabs"
          :key="item.id"
          :class="{ active: tab === item.id }"
          :aria-current="tab === item.id ? 'page' : undefined"
          @click="navigate(item.id)"
        >
          <component :is="item.icon" :size="19" /><span>{{ item.label }}</span
          ><span v-if="item.id === 'sessions' && data" class="nav-count">{{
            number(data.sessions)
          }}</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="local-status">
          <span class="status-dot"></span>本地运行 · 数据留在这台 Mac
        </div>
        <button class="settings-button" @click="settings = true">
          <Settings2 :size="17" />数据源与统计口径
        </button>
        <UpdateControl />
      </div>
    </aside>
    <div class="main-shell">
      <header class="topbar">
        <div class="breadcrumb">
          <span>工作空间</span><ChevronRight :size="14" /><strong>{{
            title
          }}</strong>
        </div>
        <div class="top-actions">
          <span class="sync-time">{{
            data?.lastScan ? "更新于 " + date(data.lastScan) : "尚未建立索引"
          }}</span
          ><button
            class="button secondary"
            :disabled="busy || status.running"
            @click="scan(tab === 'temps')"
          >
            <RefreshCw :size="16" :class="{ spinning: status.running }" />{{
              status.running ? "扫描中" : "刷新索引"
            }}
          </button>
        </div>
      </header>
      <main id="main-content">
        <div v-if="error" class="banner error" role="alert">
          {{ error
          }}<button aria-label="关闭错误提示" @click="error = ''">
            <X :size="16" />
          </button>
        </div>
        <div v-if="notice" class="banner info" role="status">
          {{ notice
          }}<button aria-label="关闭提示" @click="clearNotice">
            <X :size="16" />
          </button>
        </div>
        <div v-if="status.running" class="scan-progress" role="status">
          <div>
            <RefreshCw :size="15" class="spinning" />{{ status.phase
            }}<span>{{ status.done }} / {{ status.total }}</span>
          </div>
          <progress :value="status.done" :max="status.total || 1"></progress>
        </div>
        <div v-if="status.running || status.finished" class="scan-tools">
          <span
            >{{ status.running ? "已运行" : "用时" }}
            {{ Math.round((status.elapsedMillis || 0) / 1000) }} 秒 · 解析
            {{ status.parsed || 0 }} · 跳过 {{ status.skipped || 0 }}</span
          >
          <button
            v-if="status.running"
            class="button secondary"
            @click="cancelScan"
          >
            取消扫描
          </button>
          <button
            v-else-if="status.failedFiles?.length"
            class="button secondary"
            @click="retryScan"
          >
            重试失败日志
          </button>
          <small v-if="status.running" class="path-wrap">{{
            status.currentFile
          }}</small>
        </div>
        <details v-if="status.errors.length" class="scan-errors">
          <summary>
            {{ status.errors.length }} 项扫描未完成，已有索引保留
          </summary>
          <p v-for="item in status.errors" :key="item">{{ item }}</p>
        </details>
        <section class="page-heading">
          <div>
            <div class="eyebrow">
              {{
                tab === "overview"
                  ? "YOUR AI WORK, IN PERSPECTIVE"
                  : tab === "sessions"
                    ? "EVERY CONVERSATION, ORGANIZED"
                    : tab === "storage"
                      ? "MAKE ROOM FOR WHAT’S NEXT"
                      : tab === "history"
                        ? "RECOVERY & HISTORY"
                        : "FOLLOW THE FILE TRAIL"
              }}
            </div>
            <h1>{{ title }}</h1>
            <p>
              {{
                tab === "overview"
                  ? "了解每个项目的用量，让会话和存储井井有条。"
                  : tab === "sessions"
                    ? "找回上下文，整理历史，把注意力留给正在做的事。"
                    : tab === "storage"
                      ? "从大文件开始，看清 Codex 的空间都用在哪里。"
                      : tab === "history"
                        ? "查看清理操作和备份，在需要时恢复原始文件。"
                        : "从日志中的路径引用，追溯临时产物与项目的关联。"
              }}
            </p>
          </div>
          <button
            v-if="tab === 'overview'"
            class="button secondary"
            :disabled="!initialized || busy || !data?.sessions"
            @click="downloadReport"
          >
            <ArrowDownToLine :size="16" />导出报表</button
          ><button
            v-if="tab === 'temps'"
            class="button primary"
            :disabled="busy || status.running"
            @click="scan(true)"
          >
            <FolderSearch :size="17" />扫描临时文件
          </button>
        </section>

        <section
          v-if="initialLoading"
          class="panel initial-loading"
          role="status"
          aria-live="polite"
          aria-busy="true"
        >
          <RefreshCw :size="30" class="spinning" />
          <h2>正在读取本地索引</h2>
          <p>正在加载项目用量、会话和存储统计，请稍候…</p>
        </section>
        <section v-else-if="!initialized" class="panel initial-loading">
          <h2>数据加载失败</h2>
          <p>请检查数据目录是否可访问，然后重试。</p>
          <button class="button primary" @click="initialize">重新加载</button>
        </section>
        <template v-else>
          <template v-if="tab === 'overview'">
            <div v-if="data?.sessions" class="quality-summary">
              统计完整性：完整 {{ data.completeSessions }} · 部分
              {{ data.partialSessions }} · 无用量记录
              {{ data.noUsageSessions }}。部分记录的原因可在会话详情查看。
            </div>
            <div class="date-filter">
              <span>用量时间范围</span
              ><label
                >开始<input
                  type="date"
                  v-model="since"
                  aria-label="开始日期" /></label
              ><span>—</span
              ><label
                >结束<input
                  type="date"
                  v-model="until"
                  aria-label="结束日期" /></label
              ><button
                class="text-button"
                @click="
                  since = '';
                  until = '';
                "
              >
                全部时间</button
              ><small>会话数和存储显示当前全量</small>
            </div>
            <section v-if="data" class="stats-grid" aria-label="用量摘要">
              <article class="stat-card">
                <div><span>总 Token 用量</span><Activity :size="18" /></div>
                <strong :title="number(data?.usage.total)">{{
                  count(data?.usage.total)
                }}</strong
                ><small
                  >输入 {{ count(data?.usage.input)
                  }}<span>输出 {{ count(data?.usage.output) }}</span></small
                >
              </article>
              <article class="stat-card">
                <div><span>缓存命中占比</span><Database :size="18" /></div>
                <strong>{{ cacheRatio }}<em>%</em></strong
                ><small
                  >{{ count(data?.usage.cached) }} 缓存输入<span
                    >已包含在输入中</span
                  ></small
                >
              </article>
              <article class="stat-card">
                <div><span>已索引会话</span><FileText :size="18" /></div>
                <strong>{{ number(data?.sessions) }}</strong
                ><small
                  >分布在 {{ data?.projects.length || 0 }} 个目录<span
                    >包含保留统计</span
                  ></small
                >
              </article>
              <article class="stat-card">
                <div><span>已索引会话日志</span><HardDrive :size="18" /></div>
                <strong
                  >{{ bytes(data?.bytes).split(" ")[0]
                  }}<em>{{ bytes(data?.bytes).split(" ")[1] }}</em></strong
                ><small>不含缓存与临时文件<span>逻辑文件大小</span></small>
              </article>
            </section>
            <div v-if="data && !data.sessions" class="empty-state panel">
              <FolderSearch :size="38" />
              <h2>先给你的工作空间画张地图</h2>
              <p>
                扫描本机 Codex
                会话，建立项目、用量和存储索引。首次扫描可能需要一些时间。
              </p>
              <button
                class="button primary"
                :disabled="status.running || busy"
                @click="scan(false)"
              >
                开始扫描</button
              ><button class="button secondary" @click="settings = true">
                设置数据源
              </button>
            </div>
            <template v-else-if="data"
              ><section class="panel chart-panel">
                <div class="panel-heading">
                  <div>
                    <h2>用量趋势</h2>
                    <p>所选范围内最近 30 个有记录的日期 · Token</p>
                  </div>
                  <span class="legend"><i></i>输入 + 输出</span>
                </div>
                <div
                  class="bar-chart"
                  role="img"
                  :aria-label="
                    '最近 ' + data.days.length + ' 个有记录日期的 Token 用量'
                  "
                >
                  <div
                    v-for="day in data.days"
                    :key="day.date"
                    class="chart-column"
                  >
                    <div class="chart-track">
                      <button
                        class="chart-bar"
                        :style="{
                          height: Math.max(2, (day.total / maxDay) * 100) + '%',
                        }"
                        :aria-label="
                          day.date + '：' + number(day.total) + ' token'
                        "
                        :title="
                          day.date + ' · ' + number(day.total) + ' tokens'
                        "
                      ></button>
                    </div>
                    <span>{{ day.date.slice(5) }}</span>
                  </div>
                  <div v-if="!data.days.length" class="muted">
                    该时间范围没有用量记录
                  </div>
                </div>
              </section>
              <section class="panel">
                <div class="panel-heading">
                  <div>
                    <h2>
                      项目分布
                      <span class="count-pill">{{ projects.length }}</span>
                    </h2>
                    <p>按工作目录归类，点击项目查看会话</p>
                  </div>
                  <div class="inline-controls">
                    <label class="search-box"
                      ><Search :size="16" /><input
                        v-model="search"
                        placeholder="搜索项目目录"
                        aria-label="搜索项目目录" /></label
                    ><select v-model="projectSort" aria-label="项目排序">
                      <option value="tokens">按 Token 排序</option>
                      <option value="bytes">按空间排序</option>
                    </select>
                  </div>
                </div>
                <div class="table-scroll">
                  <table>
                    <thead>
                      <tr>
                        <th>项目 / 工作目录</th>
                        <th class="numeric">关联会话</th>
                        <th class="numeric">输入 / 缓存</th>
                        <th class="numeric">输出</th>
                        <th class="numeric">总 Token</th>
                        <th class="numeric">日志体积</th>
                        <th></th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="p in projects" :key="p.path">
                        <td>
                          <button
                            class="project-link"
                            @click="showProject(p.path)"
                          >
                            <span class="folder-icon"
                              ><Folder :size="20" /></span
                            ><span
                              ><strong>{{ p.name }}</strong
                              ><small :title="p.path">{{ p.path }}</small></span
                            >
                          </button>
                        </td>
                        <td class="numeric">{{ p.sessions }}</td>
                        <td class="numeric">
                          {{ count(p.usage.input)
                          }}<small>缓存 {{ count(p.usage.cached) }}</small>
                        </td>
                        <td class="numeric">{{ count(p.usage.output) }}</td>
                        <td class="numeric token-cell">
                          {{ count(p.usage.total) }}
                          <div class="mini-track">
                            <i
                              :style="{
                                width:
                                  (p.usage.total /
                                    (projects[0]?.usage.total || 1)) *
                                    100 +
                                  '%',
                              }"
                            ></i>
                          </div>
                        </td>
                        <td class="numeric">{{ bytes(p.bytes) }}</td>
                        <td>
                          <button
                            class="icon-button"
                            :aria-label="'查看 ' + p.name + ' 会话'"
                            @click="showProject(p.path)"
                          >
                            <ChevronRight :size="18" />
                          </button>
                        </td>
                      </tr>
                      <tr v-if="!projects.length">
                        <td colspan="7" class="empty-cell">没有匹配的项目</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </section></template
            >
            <p class="footnote">
              <ShieldCheck
                :size="15"
              />本地日志统计，不代表订阅剩余额度或账单。推理 Token
              已包含在输出中。
            </p>
          </template>

          <template v-if="tab === 'sessions'">
            <section class="panel">
              <div class="filter-bar">
                <label class="search-box grow"
                  ><Search :size="17" /><input
                    v-model="search"
                    placeholder="搜索会话标题、目录或 ID"
                    aria-label="搜索会话" /></label
                ><select v-model="project" aria-label="筛选项目">
                  <option value="">全部项目</option>
                  <option
                    v-for="p in data?.projects"
                    :key="p.path"
                    :value="p.path"
                  >
                    {{ p.name }} · {{ p.path }}
                  </option></select
                ><select v-model="state" aria-label="会话状态">
                  <option value="all">全部状态</option>
                  <option value="active">未归档</option>
                  <option value="archived">已归档</option>
                  <option value="missing">仅保留统计</option></select
                ><select v-model="sort" aria-label="会话排序">
                  <option value="updated">最近活动</option>
                  <option value="size">体积最大</option>
                  <option value="tokens">用量最高</option>
                </select>
              </div>
              <div class="selection-bar">
                <span
                  >共 {{ number(sessions.total) }} 个会话
                  <b v-if="selected.length"
                    >· 已选 {{ selected.length }} 项</b
                  ></span
                ><button
                  class="button danger-outline"
                  :disabled="!selected.length || busy || status.running"
                  @click="preview"
                >
                  <Trash2 :size="15" />预览清理
                </button>
              </div>
              <div class="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th class="check-cell">
                        <input
                          type="checkbox"
                          :checked="
                            sessions.items.filter((v) => !v.missing).length >
                              0 &&
                            sessions.items
                              .filter((v) => !v.missing)
                              .every((v) => selected.includes(v.id))
                          "
                          aria-label="选择当前页"
                          @change="togglePage"
                        />
                      </th>
                      <th>会话</th>
                      <th>项目 / 模型</th>
                      <th class="numeric">Token</th>
                      <th class="numeric">日志大小</th>
                      <th>最近活动</th>
                      <th>状态</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="v in sessions.items" :key="v.id">
                      <td>
                        <input
                          type="checkbox"
                          :checked="selected.includes(v.id)"
                          :disabled="v.missing"
                          :aria-label="'选择 ' + v.title"
                          @change="toggle(v.id)"
                        />
                      </td>
                      <td class="session-title">
                        <button @click="openDetail(v)">{{ v.title }}</button
                        ><small class="mono">{{ v.id.slice(0, 18) }}…</small>
                      </td>
                      <td>
                        <span :title="v.project">{{ basename(v.project) }}</span
                        ><small>{{ v.model || "模型未知" }}</small>
                      </td>
                      <td class="numeric">
                        {{ v.hasUsage ? count(v.usage.total) : "无记录"
                        }}<small v-if="v.completeness === 'partial'"
                          >部分统计</small
                        >
                      </td>
                      <td class="numeric">
                        {{ v.missing ? "—" : bytes(v.size) }}
                      </td>
                      <td class="nowrap muted">{{ date(v.updated) }}</td>
                      <td>
                        <span
                          class="badge"
                          :class="
                            v.missing
                              ? 'neutral'
                              : v.archived
                                ? 'amber'
                                : 'blue'
                          "
                          >{{
                            v.missing
                              ? "仅统计"
                              : v.archived
                                ? "已归档"
                                : "未归档"
                          }}</span
                        >
                      </td>
                    </tr>
                    <tr v-if="!sessions.items.length">
                      <td colspan="7" class="empty-cell">
                        没有匹配的会话。可以调整筛选或刷新索引。
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>

          <template v-if="tab === 'storage'">
            <section class="storage-hero">
              <div class="storage-symbol"><HardDrive :size="36" /></div>
              <div>
                <span>CODEX HOME · 目录占用</span>
                <h2>{{ bytes(storageTotal) }}</h2>
                <p class="mono">{{ data?.home }}</p>
              </div>
              <div class="storage-insight">
                <strong
                  >{{
                    storageTotal
                      ? Math.round(((data?.bytes || 0) / storageTotal) * 100)
                      : 0
                  }}%</strong
                ><span>空间来自会话日志</span
                ><button
                  class="text-button"
                  @click="
                    navigate('sessions');
                    sort = 'size';
                  "
                >
                  查看大体积会话 <ArrowRight :size="15" />
                </button>
              </div>
            </section>
            <section class="panel">
              <div class="panel-heading">
                <div>
                  <h2>目录占用</h2>
                  <p>逻辑文件大小；符号链接不跟随，实际可释放空间可能不同</p>
                </div>
              </div>
              <div
                v-for="(v, i) in data?.storage"
                :key="v.path"
                class="storage-row"
              >
                <div class="storage-row-title">
                  <Folder :size="19" /><span
                    ><strong>{{ basename(v.path) }}</strong
                    ><small v-if="v.error" class="error-text"
                      >扫描不完整：{{ v.error }}</small
                    ></span
                  >
                </div>
                <div class="storage-track">
                  <i
                    :style="{
                      width:
                        Math.max(0.3, (v.bytes / (storageTotal || 1)) * 100) +
                        '%',
                      background: ['#4361ee', '#8b6fd3', '#19a7a0', '#d49a32'][
                        i % 4
                      ],
                    }"
                  ></i>
                </div>
                <strong>{{ bytes(v.bytes) }}</strong>
              </div>
              <div v-if="!data?.storage.length" class="empty-cell">
                刷新索引后显示存储占用
              </div>
            </section>
            <div class="info-grid">
              <article class="panel tip-card">
                <Archive :size="23" />
                <h3>归档整理列表，删除释放空间</h3>
                <p>
                  归档仍保留会话日志。清理会话前，Atlas 保留已索引的用量数据。
                </p>
              </article>
              <article class="panel tip-card">
                <FolderSearch :size="23" />
                <h3>临时文件单独追溯</h3>
                <p>
                  临时产物可能位于系统目录，不包含在上方 Codex Home 总量中。
                </p>
                <button class="text-button" @click="navigate('temps')">
                  查看临时文件 <ArrowRight :size="15" />
                </button>
              </article>
            </div>
          </template>

          <template v-if="tab === 'temps'">
            <div class="banner info persistent">
              <ShieldCheck :size="18" /><span
                >临时文件与会话日志分别统计，系统临时目录还可能包含其他应用文件。路径引用不等于创建证明。未知归属、被占用或最近
                24 小时修改的文件不会进入清理。</span
              >
            </div>
            <section class="panel">
              <div class="filter-bar">
                <label class="search-box grow"
                  ><Search :size="17" /><input
                    v-model="search"
                    placeholder="搜索临时路径或关联项目"
                    aria-label="搜索临时文件" /></label
                ><select v-model="tempFilter" aria-label="归属筛选">
                  <option value="all">全部文件</option>
                  <option value="linked">有引用线索</option>
                  <option value="unknown">归属未知</option>
                </select>
              </div>
              <div class="selection-bar">
                <span
                  >{{ number(filteredTemps.length) }} 项 ·
                  {{ bytes(filteredTemps.reduce((a, b) => a + b.bytes, 0)) }}
                  <b v-if="selected.length"
                    >· 已选 {{ selected.length }} 项</b
                  ></span
                ><button
                  class="button danger-outline"
                  :disabled="!selected.length || busy || status.running"
                  @click="preview"
                >
                  <Trash2 :size="15" />预览移入废纸篓
                </button>
              </div>
              <div class="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th class="check-cell">
                        <input
                          type="checkbox"
                          :checked="
                            visibleTemps.length > 0 &&
                            visibleTemps.every((v) => selected.includes(v.path))
                          "
                          aria-label="选择当前页临时文件"
                          @change="togglePage"
                        />
                      </th>
                      <th>临时文件 / 目录</th>
                      <th class="numeric">大小</th>
                      <th>最后修改</th>
                      <th>关联线索</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="v in visibleTemps" :key="v.path">
                      <td>
                        <input
                          type="checkbox"
                          :checked="selected.includes(v.path)"
                          :aria-label="'选择 ' + v.name"
                          @change="toggle(v.path)"
                        />
                      </td>
                      <td class="temp-path">
                        <strong>{{ v.name }}</strong
                        ><small :title="v.path">{{ v.path }}</small
                        ><small v-if="v.error" class="error-text"
                          >扫描不完整</small
                        >
                      </td>
                      <td class="numeric">{{ bytes(v.bytes) }}</td>
                      <td class="nowrap muted">{{ date(v.modified) }}</td>
                      <td>
                        <span
                          class="badge"
                          :class="v.evidence.length ? 'blue' : 'neutral'"
                          >{{
                            v.shared
                              ? "共享目录"
                              : v.confidence === "confirmed"
                                ? "已确认"
                                : v.evidence.length
                                  ? "有引用线索"
                                  : "归属未知"
                          }}</span
                        ><small v-if="v.evidence.length"
                          >{{ basename(v.evidence[0]!.project)
                          }}{{
                            new Set(v.evidence.map((e) => e.project)).size > 1
                              ? " 等多个项目"
                              : ""
                          }}</small
                        >
                      </td>
                      <td>
                        <button class="text-button" @click="evidence = v">
                          查看依据
                        </button>
                      </td>
                    </tr>
                    <tr v-if="!visibleTemps.length">
                      <td colspan="6" class="empty-cell">
                        {{
                          temps.length
                            ? "没有匹配的文件"
                            : "点击“扫描临时文件”分析本机临时目录。扫描结果只保存在当前进程中。"
                        }}
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>
          </template>
          <RecoveryPanel
            v-if="tab === 'history'"
            @restored="perform(refresh)"
          />
          <div v-if="tab === 'sessions' || tab === 'temps'" class="pagination">
            <span>每页 50 项</span>
            <div>
              <button
                class="icon-button"
                aria-label="上一页"
                :disabled="page === 0 || busy"
                @click="page--"
              >
                <ChevronLeft :size="18" /></button
              ><span>{{ page + 1 }} / {{ totalPages }}</span
              ><button
                class="icon-button"
                aria-label="下一页"
                :disabled="page + 1 >= totalPages || busy"
                @click="page++"
              >
                <ChevronRight :size="18" />
              </button>
            </div>
          </div>
        </template>
      </main>
    </div>
  </div>
  <dialog
    ref="dialog"
    class="modal"
    aria-labelledby="modal-title"
    @cancel.prevent="closeModal"
    @click="
      (e: MouseEvent) => {
        if (e.target === dialog) closeModal();
      }
    "
  >
    <div class="modal-content">
      <div class="modal-header">
        <div>
          <div class="eyebrow">
            {{
              detail
                ? "SESSION INSPECTOR"
                : evidence
                  ? "FILE EVIDENCE"
                  : plan
                    ? "REVIEW BEFORE CLEANUP"
                    : "LOCAL DATA SOURCES"
            }}
          </div>
          <h2 id="modal-title" tabindex="-1" autofocus>
            {{
              detail
                ? "会话详情"
                : evidence
                  ? "临时文件关联依据"
                  : plan
                    ? "清理预览"
                    : "数据源与统计口径"
            }}
          </h2>
        </div>
        <button
          class="icon-button"
          aria-label="关闭对话框"
          :disabled="busy"
          @click="closeModal"
        >
          <X :size="20" />
        </button>
      </div>
      <div v-if="error" class="banner error" role="alert">{{ error }}</div>
      <template v-if="detail"
        ><h3>{{ detail.session.title }}</h3>
        <dl class="detail-meta">
          <dt>项目</dt>
          <dd>{{ detail.session.project }}</dd>
          <dt>会话 ID</dt>
          <dd>{{ detail.session.id }}</dd>
          <dt>日志</dt>
          <dd>{{ detail.session.path }}</dd>
          <dt>模型</dt>
          <dd>{{ detail.session.model || "未知" }}</dd>
          <dt>大小</dt>
          <dd>{{ bytes(detail.session.size) }}</dd>
        </dl>
        <div v-if="detail.session.warning" class="banner warning">
          {{ detail.session.warning }}
        </div>
        <div class="inline-controls">
          <button
            class="button secondary"
            :disabled="busy || detail.session.missing"
            @click="copyResume(detail.session)"
          >
            <Terminal :size="16" />复制继续会话命令</button
          ><button
            class="button secondary"
            :disabled="busy || detail.session.missing || status.running"
            @click="archive(detail.session)"
          >
            <Archive :size="16" />{{
              detail.session.archived ? "取消归档" : "归档会话"
            }}
          </button>
        </div>
        <p v-if="copyNotice" class="copy-notice" role="status">
          {{ copyNotice }}
        </p>
        <pre v-if="resume" class="command">{{ resume }}</pre>
        <h3 class="transcript-heading">对话记录</h3>
        <p class="muted">
          {{
            detail.session.completeness === "complete"
              ? "统计完整"
              : detail.session.completeness === "none"
                ? "无用量记录"
                : "部分统计"
          }}
          · 来源 {{ detail.session.provider || "codex" }} · 共
          {{ detail.total }} 条消息
        </p>
        <div class="message-controls">
          <label class="search-box"
            ><Search :size="16" /><input
              v-model="messageSearch"
              aria-label="会话内搜索"
              placeholder="搜索此会话"
              @keydown.enter="loadMessages(0)" /></label
          ><button
            class="button secondary"
            :disabled="busy || detail.session.missing"
            @click="loadMessages(0)"
          >
            搜索</button
          ><button
            class="button secondary"
            :disabled="busy || messagePage === 0"
            @click="loadMessages(messagePage - 1)"
          >
            较新消息</button
          ><button
            class="button secondary"
            :disabled="busy || (messagePage + 1) * 50 >= detail.total"
            @click="loadMessages(messagePage + 1)"
          >
            较早消息
          </button>
        </div>
        <p v-if="detail.session.missing" class="muted">
          原始日志已移除，仅保留索引与用量统计。
        </p>
        <p v-if="detail.truncated" class="muted">
          部分超长消息已截断为 24,000 字符。每页 50 条，默认从最新消息开始。
        </p>
        <article
          v-for="(m, i) in detail.messages"
          :key="i"
          class="message"
          :class="m.role"
        >
          <header>
            <strong>{{ m.role === "user" ? "你" : "Codex" }}</strong
            ><time>{{ date(m.time) }}</time>
          </header>
          <pre>{{ m.text }}</pre>
        </article></template
      >
      <template v-if="evidence"
        ><p class="mono path-wrap">{{ evidence.path }}</p>
        <p class="muted">
          {{ bytes(evidence.bytes) }} · {{ date(evidence.modified) }}
        </p>
        <div v-if="!evidence.evidence.length" class="empty-state">
          <FolderSearch :size="30" />
          <h3>尚未找到日志引用</h3>
          <p>名称和修改时间不足以确定所属项目。此项不会进入批量清理。</p>
        </div>
        <div v-for="(e, i) in evidence.evidence" :key="i" class="evidence-card">
          <span class="badge blue">{{ e.kind }}</span>
          <h3>{{ e.project }}</h3>
          <p class="mono">{{ e.sessionId }}</p>
          <p class="path-wrap">{{ e.path }}</p>
        </div>
        <div
          v-if="!evidence.shared && !evidence.link && !evidence.error"
          class="ownership-confirm"
        >
          <p>
            只有引用线索时，普通清理会被阻止。可核实后确认所属项目，或在清理预览中选择强制删除。归属确认只对当前扫描有效。
          </p>
          <select v-model="confirmedProject" aria-label="确认临时文件所属项目">
            <option value="">选择所属项目</option>
            <option v-for="p in data?.projects" :key="p.path" :value="p.path">
              {{ p.path }}
            </option></select
          ><button
            class="button secondary"
            :disabled="!confirmedProject || busy"
            @click="confirmTemp"
          >
            确认归属
          </button>
        </div>
        <p v-if="evidence.cleanupBlocked" class="banner warning">
          {{ evidence.cleanupBlocked }}
        </p>
        <p v-if="evidence.evidence.length" class="muted">
          最多显示 50
          条引用。命令参数或工具输出可能只是查询此路径，不能据此认定它由该会话创建。
        </p></template
      >
      <template v-if="plan"
        ><div class="cleanup-summary">
          <Trash2 :size="26" />
          <div>
            <strong>{{ bytes(plan.bytes) }}</strong>
            <p>
              {{
                plan.kind === "session"
                  ? plan.backup
                    ? "计划归档并压缩备份后移除原日志，可在清理历史恢复。备份仍占用部分空间。"
                    : "计划永久删除会话日志，保留已索引的用量统计，无法恢复对话。"
                  : "计划移入 macOS 废纸篓；清空废纸篓后才会释放空间。"
              }}
            </p>
          </div>
        </div>
        <label v-if="plan.kind === 'temp' && !results.length" class="confirm-label">
          <input type="checkbox" :checked="plan.force" :disabled="busy" @change="setForceCleanup" />
          强制删除（无需标记归属）
        </label>
        <p v-if="plan.force" class="banner warning">
          已跳过归属标记和共享目录限制，请逐项核对路径。共享目录可能影响多个项目；仍移入废纸篓。被占用、近期修改、符号链接、扫描不完整或路径发生变化的项目仍会被阻止。
        </p>
        <div class="cleanup-list">
          <article v-for="item in plan.items" :key="item.id">
            <span class="badge" :class="item.blocked ? 'amber' : 'blue'">{{
              item.blocked ? "已阻止" : "可清理"
            }}</span>
            <div>
              <strong class="path-wrap">{{ item.path || item.id }}</strong>
              <p>{{ item.blocked || bytes(item.bytes) }}</p>
            </div>
          </article>
        </div>
        <p v-if="!results.length && !plan.items.some(item => !item.blocked)" class="banner warning" role="status">
          当前没有可清理项，执行按钮不可用。请查看上方每项的阻止原因。
        </p>
        <template v-if="!results.length"
          ><label class="confirm-label" for="confirmation"
            >输入「{{ plan.force ? "确认强制清理" : "确认清理" }}」以执行以上可清理项</label
          ><input
            id="confirmation"
            v-model="confirmation"
            autocomplete="off"
            :placeholder="plan.force ? '确认强制清理' : '确认清理'"
          />
          <div class="modal-footer">
            <button
              class="button secondary"
              :disabled="busy"
              @click="closeModal"
            >
              取消</button
            ><button
              class="button danger"
              :disabled="
                busy ||
                confirmation !== (plan.force ? '确认强制清理' : '确认清理') ||
                !plan.items.some((i) => !i.blocked)
              "
              @click="execute"
            >
              {{ busy ? "正在检查并清理…" : "执行清理" }}
            </button>
          </div></template
        >
        <div v-else class="results">
          <article v-for="r in results" :key="r.id">
            <Check v-if="r.success" :size="18" /><X v-else :size="18" /><span>{{
              r.message
            }}</span>
          </article>
          <button class="button primary" @click="closeModal">完成</button>
        </div></template
      >
      <template v-if="settings"
        ><dl class="detail-meta">
          <dt>独立数据库</dt>
          <dd>{{ data?.database }}</dd>
        </dl>
        <PreferencesPanel @saved="perform(refresh)"
      /></template>
    </div>
  </dialog>
</template>
