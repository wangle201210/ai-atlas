<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";
import { FolderOpen, RefreshCw, Download, ExternalLink } from "lucide-vue-next";
import { Browser } from "@wailsio/runtime";
import type {
  Settings,
  EnvironmentStatus,
} from "../../bindings/ai-atlas/internal/atlas/models";
const backend = () => import("../../bindings/ai-atlas/internal/atlas/service");
const emit = defineEmits<{ saved: [] }>();
const config = ref<Settings>(),
  environment = ref<EnvironmentStatus>(),
  extra = ref(""),
  excluded = ref(""),
  busy = ref(false),
  error = ref(""),
  saved = ref(false);
const exportNotice = ref("");
let exportNoticeTimer: ReturnType<typeof setTimeout> | undefined;
onUnmounted(() => clearTimeout(exportNoticeTimer));
async function run(fn: () => Promise<void>) {
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
async function inspect() {
  await run(async () => {
    environment.value = await (await backend()).Environment();
  });
}
async function choose(field: "home" | "cli") {
  await run(async () => {
    const b = await backend();
    const path = await (field === "home" ? b.ChooseDirectory() : b.ChooseCLI());
    if (path && config.value) config.value[field] = path;
  });
}
async function save() {
  await run(async () => {
    if (!config.value) return;
    await (
      await backend()
    ).SaveSettings({
      ...config.value,
      tempDirectories: extra.value.split("\n").filter(Boolean),
      excludedDirectories: excluded.value.split("\n").filter(Boolean),
    });
    saved.value = true;
    emit("saved");
    environment.value = await (await backend()).Environment();
  });
}
async function diagnostics() {
  clearTimeout(exportNoticeTimer);
  exportNotice.value = "";
  await run(async () => {
    const path = await (await backend()).ExportDiagnostics();
    if (path) {
      exportNotice.value = `诊断已保存至：${path}`;
      exportNoticeTimer = setTimeout(() => (exportNotice.value = ""), 10000);
    }
  });
}
onMounted(() =>
  run(async () => {
    const b = await backend();
    config.value = await b.Settings();
    extra.value = (config.value.tempDirectories ?? []).join("\n");
    excluded.value = (config.value.excludedDirectories ?? []).join("\n");
    environment.value = await b.Environment();
  }),
);
</script>
<template>
  <div class="preferences">
    <p class="muted">
      选择本机数据源和扫描范围。设置保存在独立数据库中，保存后立即生效。
    </p>
    <p v-if="error" class="banner error" role="alert">{{ error }}</p>
    <p v-if="saved" class="banner info" role="status">
      设置已保存。刷新索引后读取新数据源。
    </p>
    <template v-if="config">
      <label for="source-home">Codex 数据目录</label>
      <div class="setting-path">
        <input id="source-home" v-model="config.home" /><button
          class="button secondary"
          :disabled="busy"
          @click="choose('home')"
        >
          <FolderOpen :size="16" />选择目录
        </button>
      </div>
      <label for="codex-cli">Codex CLI 路径（留空自动发现）</label>
      <div class="setting-path">
        <input
          id="codex-cli"
          v-model="config.cli"
          placeholder="自动发现"
        /><button
          class="button secondary"
          :disabled="busy"
          @click="choose('cli')"
        >
          选择文件
        </button>
      </div>
      <p class="muted cli-path" aria-live="polite">
        当前生效路径（已保存配置）：<code>{{
          environment?.cliPath || (environment ? "未找到 Codex CLI" : "正在检测…")
        }}</code>
      </p>
      <div v-if="environment" class="environment-result">
        <span>{{
          environment.homeExists
            ? "数据目录可访问"
            : "数据目录不存在，请选择正确目录"
        }}</span
        ><span>{{
          environment.cliVersion || environment.error || "未检测到 CLI"
        }}</span
        ><button class="text-button" :disabled="busy" @click="inspect">
          <RefreshCw :size="13" />重新检测已保存配置
        </button>
      </div>
      <label class="check-label"
        ><input
          type="checkbox"
          v-model="config.scanSystemTemp"
        />扫描系统临时目录（/tmp 和 TMPDIR）</label
      >
      <p class="muted">
        默认开启。系统临时目录包含其他应用的文件，默认只展示有会话引用线索的项目；此开关不影响会话日志统计。
      </p>
      <label for="extra-temp">额外临时目录（每行一个）</label
      ><textarea
        id="extra-temp"
        v-model="extra"
        rows="3"
        placeholder="/path/to/project/tmp"
      />
      <label for="exclude-paths">排除目录（每行一个）</label
      ><textarea id="exclude-paths" v-model="excluded" rows="3" />
      <label class="check-label"
        ><input
          type="checkbox"
          v-model="config.backupSessions"
        />清理会话时先压缩备份，保留恢复能力</label
      >
      <p class="muted">
        开启时先归档、备份，再移除原日志；备份仍占用部分空间。关闭后使用 Codex
        永久删除会话，无法恢复对话。
      </p>
      <button class="button primary" :disabled="busy" @click="save">
        {{ busy ? "处理中…" : "保存设置" }}
      </button>
    </template>
    <div class="diagnostic-actions">
      <button class="button secondary" :disabled="busy" @click="diagnostics">
        <Download :size="15" />导出脱敏诊断</button
      ><button
        class="text-button"
        @click="
          Browser.OpenURL('https://github.com/wangle201210/ai-atlas/issues')
        "
      >
        <ExternalLink :size="14" />反馈问题
      </button>
    </div>
    <p v-if="exportNotice" class="banner info cli-path" role="status">
      {{ exportNotice }}
    </p>
    <p class="muted">
      诊断仅包含计数和任务状态，不包含路径、消息、标题或会话
      ID。会话数据留在本机；仅检查更新和打开反馈入口时访问 GitHub。
    </p>
  </div>
</template>
<style scoped>
.preferences > label {
  display: block;
  margin: 20px 0 9px;
  font-size: 13px;
  font-weight: 550;
}
.setting-path {
  display: flex;
  gap: 10px;
}
.setting-path input {
  flex: 1;
  min-width: 0;
}
.cli-path {
  overflow-wrap: anywhere;
}
.cli-path code {
  user-select: text;
}
.preferences textarea {
  width: 100%;
  resize: vertical;
  border: 1px solid #dce3ed;
  border-radius: 7px;
  padding: 10px;
  font: inherit;
  font-size: 12px;
}
.preferences .check-label {
  display: flex;
  gap: 10px;
  align-items: center;
}
.preferences .muted {
  line-height: 1.8;
  margin: 10px 0;
}
.preferences > .primary {
  margin-top: 16px;
}
.environment-result {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 12px;
  background: #f5f7fc;
  margin-top: 12px;
  font-size: 12px;
}
.diagnostic-actions {
  display: flex;
  gap: 18px;
  align-items: center;
  border-top: 1px solid var(--line);
  padding-top: 20px;
  margin-top: 24px;
}
</style>
