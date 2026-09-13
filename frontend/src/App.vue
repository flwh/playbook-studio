<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Events } from "@wailsio/runtime";
import Icon from "./components/Icon.vue";
import TitleBar from "./components/TitleBar.vue";
import OpenScreen from "./components/OpenScreen.vue";
import SideBar from "./components/SideBar.vue";
import EditorPane from "./components/EditorPane.vue";
import RightPanel from "./components/RightPanel.vue";
import { state, initApp, closeBundle, reloadBundle, runValidate, saveBundle, reveal } from "./store";
import { PlaybookService as PB } from "./api";

const dirtyCount = computed(() => (state.bundle?.dirty ?? []).length);
const archiveName = computed(() => (state.bundle?.archive ?? "").split(/[\\/]/).pop() ?? "");
const actionCount = computed(() => state.task?.actions?.length ?? 0);
const confirmClose = ref(false);

function onKey(e: KeyboardEvent) {
  if (!state.bundle) return;
  if (e.ctrlKey && !e.shiftKey && e.key.toLowerCase() === "s") {
    e.preventDefault();
    void saveBundle(false);
  } else if (e.ctrlKey && e.shiftKey && e.key.toLowerCase() === "s") {
    e.preventDefault();
    void saveBundle(true);
  } else if (e.key === "F5") {
    e.preventDefault();
    void runValidate();
  }
}

function openConfirmClose() {
  confirmClose.value = true;
}

function onPbClose() {
  openConfirmClose();
}

async function forceClose() {
  confirmClose.value = false;
  try {
    await PB.WindowForceClose();
  } catch {
    /* ignore */
  }
}

async function saveAndClose() {
  await saveBundle(false);
  confirmClose.value = false;
  if (dirtyCount.value === 0) {
    try {
      await PB.WindowForceClose();
    } catch {
      /* ignore */
    }
  }
}

onMounted(() => {
  window.addEventListener("keydown", onKey);
  window.addEventListener("pb-close-request", onPbClose);
  try {
    Events.On("app:close-request", onPbClose);
  } catch {
    /* 忽略 */
  }
  void initApp();
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKey);
  window.removeEventListener("pb-close-request", onPbClose);
});
</script>

<template>
  <div class="app" :class="{ 'no-bundle': !state.bundle }">
    <TitleBar>
      <template v-if="state.bundle">
        <button class="ghost" @click="reloadBundle" title="丢弃修改并重新解包">
          <Icon name="refresh" :size="14" /> 重新载入
        </button>
        <button class="ghost" @click="closeBundle" title="关闭当前 playbook">
          <Icon name="close" :size="14" /> 关闭
        </button>

        <div class="file-chip" :title="state.bundle.archive">
          <Icon name="layers" :size="13" />
          <b>{{ state.bundle.summary?.title || archiveName }}</b>
          <span>v{{ state.bundle.summary?.version || "?" }}</span>
          <span class="dot-dirty" v-if="dirtyCount" :title="`${dirtyCount} 个文件未保存`"></span>
        </div>

        <div style="flex: 1; min-width: 12px"></div>

        <button @click="runValidate" title="F5"><Icon name="shield" :size="14" /> 校验</button>
        <button @click="saveBundle(true)" title="Ctrl+Shift+S"><Icon name="download" :size="14" /> 另存为</button>
        <button class="primary" :disabled="!dirtyCount" @click="saveBundle(false)" title="Ctrl+S">
          <Icon name="save" :size="14" /> 保存<span v-if="dirtyCount"> ({{ dirtyCount }})</span>
        </button>
      </template>
    </TitleBar>

    <OpenScreen v-if="!state.bundle" />

    <template v-else>
      <div class="body">
        <SideBar />
        <EditorPane />
        <RightPanel />
      </div>

      <div class="statusbar">
        <span :class="state.status.sevenZipOk ? 'ok' : 'bad'">
          <Icon name="box" :size="12" />
          {{
            !state.status.sevenZipOk
              ? "7z 缺失"
              : state.status.sevenZipSource === "builtin"
                ? "7z 内置"
                : "7z 外部"
          }}
        </span>
        <span class="mono" style="font-size: 11px; max-width: 400px; overflow: hidden; text-overflow: ellipsis">
          {{ state.status.sevenZip }}
        </span>
        <span class="sep">|</span>
        <span class="mono" style="font-size: 11px; cursor: pointer" @click="reveal(state.bundle.workDir)" title="点击在资源管理器中打开">
          解包目录: {{ state.bundle.workDir }}
        </span>
        <span class="sep">|</span>
        <span v-if="state.currentPath">任务: {{ state.currentPath }} · {{ actionCount }} 个动作</span>
        <span v-else-if="state.filePath">文件: {{ state.filePath }}</span>
        <span v-else>未选择内容</span>
        <div class="spacer" style="flex: 1"></div>
        <span v-if="dirtyCount" style="color: var(--warn)">{{ dirtyCount }} 个文件待保存</span>
        <span v-else class="ok">已同步</span>
      </div>
    </template>
  </div>

  <div class="modal-mask" v-if="confirmClose">
    <div class="modal">
      <h3><Icon name="alert" :size="16" /> 有未保存的修改</h3>
      <p>以下 {{ dirtyCount }} 个文件还没有写回 .apbx：</p>
      <div class="dirty-list mono">
        <div v-for="f in state.bundle?.dirty ?? []" :key="f">{{ f }}</div>
      </div>
      <div class="modal-btns">
        <button @click="confirmClose = false">取消</button>
        <button class="danger" @click="forceClose">放弃修改并退出</button>
        <button class="primary" @click="saveAndClose" :disabled="state.busy">保存并退出</button>
      </div>
    </div>
  </div>

  <Transition name="fade">
    <div class="toast" v-if="state.toast" :class="state.toastKind">{{ state.toast }}</div>
  </Transition>
</template>
