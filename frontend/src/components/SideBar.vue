<script setup lang="ts">
import { computed } from "vue";
import Icon from "./Icon.vue";
import TreeNode from "./TreeNode.vue";
import { state, selectTask, openRawFile } from "../store";
import type { TreeNode as TN } from "../../bindings/playbookstudio/internal/playbook/models";

const tree = computed(() => ((state.bundle?.tree ?? []) as (TN | null)[]).filter(Boolean) as TN[]);
const files = computed(() => state.bundle?.files ?? []);
const dirty = computed(() => new Set(state.bundle?.dirty ?? []));

function fileIcon(p: string): string {
  const l = p.toLowerCase();
  if (l.endsWith(".yml") || l.endsWith(".yaml")) return "code";
  if (l.endsWith(".conf")) return "wrench";
  if (l.endsWith(".ps1") || l.endsWith(".cmd") || l.endsWith(".exe")) return "terminal";
  if (l.endsWith(".xml") || l.endsWith(".json")) return "list";
  if (l.endsWith(".png") || l.endsWith(".jpg") || l.endsWith(".ico")) return "box";
  return "file";
}

function sizeText(n: number): string {
  if (n < 1024) return n + "B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(0) + "K";
  return (n / 1024 / 1024).toFixed(1) + "M";
}

async function onFile(rel: string) {
  if (rel.toLowerCase().endsWith(".yml") || rel.toLowerCase().endsWith(".yaml")) {
    if ((state.bundle?.taskPaths ?? []).includes(rel)) {
      await selectTask(rel);
      return;
    }
  }
  await openRawFile(rel);
}
</script>

<template>
  <div class="panel">
    <div class="panel-head">
      <div class="tabs">
        <button :class="{ active: state.leftTab === 'tree' }" @click="state.leftTab = 'tree'">任务树</button>
        <button :class="{ active: state.leftTab === 'files' }" @click="state.leftTab = 'files'">文件</button>
      </div>
      <div style="flex: 1"></div>
      <span class="mono" style="font-size: 11px">{{ files.length }}</span>
    </div>

    <div class="panel-body" v-if="state.leftTab === 'tree'">
      <div v-if="!tree.length" class="empty">没有解析到任务树</div>
      <TreeNode v-for="(n, i) in tree" :key="i" :node="n" />
    </div>

    <div class="panel-body flush" v-else>
      <div
        class="file-row"
        v-for="f in files"
        :key="f.path"
        :class="{ active: state.currentPath === f.path || state.filePath === f.path }"
        @click="onFile(f.path)"
        :title="f.path"
      >
        <Icon :name="fileIcon(f.path)" :size="13" />
        <span class="fname">{{ f.path }}</span>
        <span class="dirty" v-if="dirty.has(f.path)">●</span>
        <span class="fsize">{{ sizeText(f.size) }}</span>
      </div>
    </div>
  </div>
</template>
