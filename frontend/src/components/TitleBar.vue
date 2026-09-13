<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import Icon from "./Icon.vue";
import { PlaybookService as PB } from "../api";

const maximised = ref(false);

async function sync() {
  try {
    maximised.value = await PB.WindowIsMaximised();
  } catch {
    /* server 模式忽略 */
  }
}

async function minimise() {
  try {
    await PB.WindowMinimise();
  } catch {
    /* ignore */
  }
}

async function toggle() {
  try {
    maximised.value = await PB.WindowToggleMaximise();
  } catch {
    /* ignore */
  }
}

async function close() {
  try {
    const r = await PB.WindowCloseRequest();
    if (r === "dirty") window.dispatchEvent(new CustomEvent("pb-close-request"));
  } catch {
    /* ignore */
  }
}

// 拖拽由 Wails 运行时实现：标题栏在 CSS 里声明 --wails-draggable: drag，
// 运行时的 drag.js 会在按住并移动时调用原生拖动（自带贴边吸附）。
// Windows 下双击最大化需要自己处理，见模板里的 @dblclick。
onMounted(() => {
  window.addEventListener("resize", sync);
  window.addEventListener("focus", sync);
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", sync);
  window.removeEventListener("focus", sync);
});
</script>

<template>
  <div class="titlebar">
    <div class="brand tb-drag-area" @dblclick="toggle">
      <div class="logo">PB</div>
      <div class="brand-text">
        Playbook Studio
        <small>AME Playbook 工作流编辑器</small>
      </div>
    </div>

    <div class="tb-actions">
      <slot />
    </div>

    <div class="tb-drag tb-drag-area" title="拖动窗口" @dblclick="toggle"></div>

    <div class="tb-winbtns">
      <button class="wb" title="最小化" @click="minimise"><Icon name="min" :size="14" /></button>
      <button class="wb" :title="maximised ? '还原' : '最大化'" @click="toggle">
        <Icon :name="maximised ? 'restore' : 'max'" :size="13" />
      </button>
      <button class="wb close" title="关闭" @click="close"><Icon name="close" :size="14" /></button>
    </div>
  </div>
</template>
