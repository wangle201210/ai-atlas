<script setup lang="ts">
import { onMounted, ref } from "vue";
import type { Recovery } from "../../bindings/ai-atlas/internal/atlas/models";
const backend = () => import("../../bindings/ai-atlas/internal/atlas/service");
const emit = defineEmits<{ restored: [] }>();
const items = ref<Recovery[]>([]),
  error = ref(""),
  notice = ref(""),
  busy = ref(false),
  selected = ref<Recovery>(),
  confirmation = ref("");
const size = (n: number) => (n / 1024 / 1024).toFixed(1) + " MiB";
async function load() {
  try {
    items.value = (await (await backend()).Recoveries()) ?? [];
  } catch (e) {
    error.value = String(e);
  }
}
async function restore() {
  if (!selected.value) return;
  busy.value = true;
  error.value = "";
  try {
    await (
      await backend()
    ).RestoreRecovery(selected.value.id, confirmation.value);
    selected.value = undefined;
    confirmation.value = "";
    notice.value = "已恢复到原路径";
    await load();
    emit("restored");
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
onMounted(load);
</script>
<template>
  <section class="panel recovery-panel">
    <div class="panel-heading">
      <div>
        <h2>清理历史与恢复</h2>
        <p>最近 300 项。备份保留在本机，恢复不会覆盖已存在的文件。</p>
      </div>
      <button class="button secondary" :disabled="busy" @click="load">
        刷新记录
      </button>
    </div>
    <p v-if="error" class="banner error" role="alert">{{ error }}</p>
    <p v-if="notice" class="banner info" role="status">{{ notice }}</p>
    <div v-if="selected" class="recovery-confirm">
      <h3>恢复预览</h3>
      <p class="path-wrap">来源：{{ selected.stored }}</p>
      <p class="path-wrap">恢复到：{{ selected.source }}</p>
      <p>
        原始大小 {{ size(selected.size) }}。恢复会话时会同步恢复原来的归档状态。
      </p>
      <label for="restore-confirm">输入「确认恢复」</label
      ><input id="restore-confirm" v-model="confirmation" />
      <div class="inline-controls">
        <button
          class="button secondary"
          :disabled="busy"
          @click="selected = undefined"
        >
          取消</button
        ><button
          class="button primary"
          :disabled="busy || confirmation !== '确认恢复'"
          @click="restore"
        >
          {{ busy ? "正在恢复…" : "执行恢复" }}
        </button>
      </div>
    </div>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>清理项</th>
            <th>方式 / 状态</th>
            <th>保留备份</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in items" :key="r.id">
            <td class="path-wrap">
              {{ r.source
              }}<small>{{ new Date(r.created).toLocaleString() }}</small
              ><small v-if="r.error" class="error-text">{{ r.error }}</small>
            </td>
            <td>
              {{
                r.kind === "session"
                  ? "压缩备份"
                  : r.kind === "permanent"
                    ? "永久删除"
                    : "废纸篓"
              }}<small>{{
                r.state === "restored"
                  ? "已恢复"
                  : r.state === "ready"
                    ? "可恢复"
                    : r.state === "deleted"
                      ? "不可恢复"
                      : r.state === "error"
                        ? "操作未完成"
                        : "处理中"
              }}</small>
            </td>
            <td>
              {{ size(r.kind === "session" ? r.storedBytes : r.size)
              }}<small class="path-wrap">{{ r.stored }}</small>
            </td>
            <td>
              <button
                class="button secondary"
                :disabled="busy || !['ready', 'error'].includes(r.state)"
                @click="
                  selected = r;
                  confirmation = '';
                  error = '';
                "
              >
                恢复…
              </button>
            </td>
          </tr>
          <tr v-if="!items.length">
            <td colspan="4" class="empty-cell">
              还没有可恢复的清理记录。会话备份默认开启。
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
<style scoped>
.recovery-panel > .banner {
  margin: 16px;
}
.recovery-panel td {
  max-width: 340px;
}
.recovery-confirm {
  padding: 20px;
  margin: 16px;
  background: #f4f7ff;
  border-radius: 9px;
}
.recovery-confirm p {
  font-size: 12px;
  line-height: 1.8;
  margin: 10px 0;
}
.recovery-confirm label {
  font-size: 12px;
  margin-right: 12px;
}
.recovery-confirm .inline-controls {
  margin-top: 14px;
}
</style>
