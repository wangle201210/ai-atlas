<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import {
  ArrowDownToLine,
  CheckCircle2,
  ExternalLink,
  RefreshCw,
  RotateCw,
  X,
} from "lucide-vue-next";
import { Browser } from "@wailsio/runtime";
import type { State } from "../../bindings/ai-atlas/internal/updates/models";
const backend = () =>
  import("../../bindings/ai-atlas/internal/updates/service");
const state = ref<State>({
  currentVersion: "",
  latestVersion: "",
  phase: "idle",
  notes: "",
  releaseName: "",
  releaseURL: "https://github.com/wangle201210/ai-atlas/releases",
  checkedAt: "",
  written: 0,
  total: 0,
  canInstall: false,
  error: "",
});
const dialog = ref<HTMLDialogElement>(),
  opened = ref(false),
  pending = ref(false);
const active = computed(() =>
  ["checking", "downloading", "verifying", "installing", "restarting"].includes(
    state.value.phase,
  ),
);
const percent = computed(() =>
  state.value.total > 0
    ? Math.min(100, Math.round((state.value.written / state.value.total) * 100))
    : 0,
);
const headings: Record<string, string> = {
  idle: "检查 AI Atlas 更新",
  checking: "正在检查新版本",
  current: "当前没有可用更新",
  available: "发现新版本",
  downloading: "正在下载更新",
  verifying: "正在校验更新包",
  installing: "正在准备安装",
  ready: "更新已准备好",
  restarting: "正在重启安装",
  error: "更新未完成",
};
let timer: ReturnType<typeof setInterval> | undefined,
  refreshing = false;
async function refresh() {
  if (refreshing) return;
  refreshing = true;
  try {
    state.value = await (await backend()).Status();
  } catch (e) {
    state.value.error = String(e);
  } finally {
    refreshing = false;
  }
}
async function action(kind: "Check" | "Download" | "Restart") {
  pending.value = true;
  try {
    await (await backend())[kind]();
    await refresh();
  } catch (e) {
    state.value.error = String(e);
  } finally {
    pending.value = false;
  }
}
async function open() {
  opened.value = true;
  await nextTick();
  if (!dialog.value?.open) dialog.value?.showModal();
  await refresh();
  if (!active.value && state.value.phase !== "ready") await action("Check");
}
function close() {
  opened.value = false;
  dialog.value?.close();
}
async function openReleases() {
  try {
    await Browser.OpenURL(state.value.releaseURL);
  } catch (e) {
    state.value.error = String(e);
  }
}
const size = (n: number) => (n / 1024 / 1024).toFixed(1) + " MiB";
onMounted(() => {
  void refresh();
  timer = setInterval(() => {
    if (opened.value || active.value) void refresh();
  }, 750);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <button
    class="version version-button"
    aria-label="检查 AI Atlas 更新"
    title="点击检查 AI Atlas 的 Release 更新"
    @click="open"
  >
    <span>AI ATLAS</span
    ><span class="version-value"
      ><span
        v-if="['available', 'ready'].includes(state.phase)"
        class="update-dot"
      ></span
      >{{ state.currentVersion ? "v" + state.currentVersion : "检查更新"
      }}<RefreshCw :size="11" :class="{ spinning: active }"
    /></span>
  </button>
  <dialog
    ref="dialog"
    class="modal update-modal"
    aria-labelledby="update-title"
    @cancel.prevent="close"
    @click="
      (e: MouseEvent) => {
        if (e.target === dialog) close();
      }
    "
  >
    <div class="modal-content">
      <div class="modal-header">
        <div>
          <div class="eyebrow">AI ATLAS · SOFTWARE UPDATE</div>
          <h2 id="update-title">{{ headings[state.phase] || "检查更新" }}</h2>
        </div>
        <button class="icon-button" aria-label="关闭更新窗口" @click="close">
          <X :size="20" />
        </button>
      </div>
      <div class="update-versions">
        <span
          >当前版本
          <strong>{{
            state.currentVersion ? "v" + state.currentVersion : "正在读取…"
          }}</strong></span
        ><span v-if="state.latestVersion"
          >新版本 <strong>v{{ state.latestVersion }}</strong></span
        >
      </div>
      <div v-if="state.error" class="banner error" role="alert">
        {{ state.error }}
      </div>
      <p
        v-if="state.phase === 'checking'"
        class="update-description"
        role="status"
      >
        正在检查 wangle201210/ai-atlas 的 GitHub Releases…
      </p>
      <div
        v-if="state.phase === 'current'"
        class="update-current"
        role="status"
      >
        <CheckCircle2 :size="30" />
        <p>GitHub Releases 暂未发布比当前版本更高的正式版本。</p>
      </div>
      <template v-if="state.latestVersion">
        <div v-if="state.notes" class="release-notes">
          <h3>{{ state.releaseName || "更新说明" }}</h3>
          <pre>{{ state.notes }}</pre>
        </div>
        <p v-else class="update-description">此版本未提供更新说明。</p>
        <p v-if="!state.canInstall" class="update-description">
          当前运行方式不支持自动安装。请使用已打包的 macOS 应用，或前往 Release
          页面手动下载。
        </p>
      </template>
      <div
        v-if="['downloading', 'verifying', 'installing'].includes(state.phase)"
        class="update-progress"
        role="status"
      >
        <div>
          <span>{{
            state.phase === "downloading"
              ? "下载更新包"
              : state.phase === "verifying"
                ? "验证 SHA-256 校验值"
                : "准备应用文件"
          }}</span
          ><span>{{ size(state.written) }} / {{ size(state.total) }}</span>
        </div>
        <progress
          :value="percent"
          max="100"
          :aria-label="'更新下载进度 ' + percent + '%'"
        />
        <p>可以关闭此窗口，下载会继续进行。</p>
      </div>
      <p
        v-if="state.phase === 'ready'"
        class="update-description"
        role="status"
      >
        更新包已下载并通过校验。点击「重启并安装」后会退出 AI
        Atlas、替换应用并重新打开；本地索引会保留。
      </p>
      <p
        v-if="state.phase === 'restarting'"
        class="update-description"
        role="status"
      >
        正在重启 AI Atlas，请稍候…
      </p>
      <div class="modal-footer update-footer">
        <button class="text-button" @click="openReleases">
          <ExternalLink :size="14" />查看 GitHub Releases
        </button>
        <button
          v-if="state.phase === 'available'"
          class="button primary"
          :disabled="pending || !state.canInstall"
          @click="action('Download')"
        >
          <ArrowDownToLine :size="16" />下载更新
        </button>
        <button
          v-else-if="state.phase === 'ready'"
          class="button primary"
          :disabled="pending || !state.canInstall"
          @click="action('Restart')"
        >
          <RotateCw :size="16" />重启并安装
        </button>
        <button
          v-else-if="state.phase === 'error'"
          class="button primary"
          :disabled="pending"
          @click="action('Check')"
        >
          <RefreshCw :size="16" />重新检查
        </button>
        <button v-else class="button secondary" @click="close">
          {{ active ? "后台继续" : "关闭" }}
        </button>
      </div>
    </div>
  </dialog>
</template>

<style scoped>
.version-button {
  width: calc(100% - 18px);
  padding: 10px 0;
  align-items: center;
  text-align: left;
  border-radius: 6px;
  transition: color 0.15s;
}
.version-button:hover {
  color: #dce5ff;
}
.version-value {
  display: flex;
  gap: 7px;
  align-items: center;
}
.update-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #79d8b4;
}
.update-modal {
  width: min(550px, calc(100vw - 32px));
  text-align: left;
  letter-spacing: normal;
}
.update-versions {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  font-size: 12px;
  color: var(--muted);
  padding: 16px 0 20px;
}
.update-versions strong {
  margin-left: 8px;
  color: #26364f;
}
.update-description {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.9;
  margin: 12px 0;
}
.update-current {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 24px 0;
  color: #3b7b6f;
}
.update-current p {
  font-size: 13px;
  line-height: 1.8;
  color: var(--muted);
}
.release-notes {
  padding: 18px;
  background: #f7f9fc;
  border: 1px solid var(--line);
  border-radius: 8px;
  max-height: 300px;
  overflow: auto;
}
.release-notes h3 {
  font-size: 14px;
  margin-bottom: 10px;
}
.release-notes pre {
  font: inherit;
  font-size: 12px;
  line-height: 1.8;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  margin: 0;
}
.update-progress {
  margin: 20px 0;
  font-size: 12px;
  color: var(--muted);
}
.update-progress > div {
  display: flex;
  justify-content: space-between;
  gap: 12px;
}
.update-progress progress {
  width: 100%;
  height: 7px;
  accent-color: var(--primary);
  margin: 14px 0;
}
.update-progress p {
  font-size: 11px;
}
.update-footer {
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
}
@media (prefers-reduced-motion: reduce) {
  .version-button {
    transition: none;
  }
}
</style>
