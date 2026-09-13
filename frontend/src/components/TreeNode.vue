<script setup lang="ts">
import { computed, ref } from "vue";
import Icon from "./Icon.vue";
import { state, selectTask, notify } from "../store";
import type { TreeNode } from "../../bindings/playbookstudio/internal/playbook/models";

const props = defineProps<{ node: TreeNode }>();

const expanded = ref(true);
const children = computed(() => (props.node.children ?? []).filter(Boolean) as TreeNode[]);
const active = computed(() => !!props.node.path && state.currentPath === props.node.path);

function baseName(p: string): string {
  const parts = p.split(/[\\/]/);
  return parts[parts.length - 1] ?? p;
}

async function click() {
  if (props.node.missing) {
    notify(`引用的任务不存在: ${props.node.include}`, "err");
    return;
  }
  await selectTask(props.node.path);
}
</script>

<template>
  <div class="tree-node">
    <div class="tree-row" :class="{ active, missing: node.missing }" @click="click">
      <span class="caret" :class="{ open: expanded }" @click.stop="expanded = !expanded">
        {{ children.length ? "▶" : "" }}
      </span>
      <Icon :name="node.missing ? 'alert' : 'file'" :size="13" />
      <span class="label">{{ node.title || baseName(node.include || node.path) }}</span>
      <span class="count" v-if="node.actionCount">{{ node.actionCount }}</span>
    </div>
    <div class="tree-children" v-if="expanded && children.length">
      <TreeNode v-for="(c, i) in children" :key="(c.path || c.include) + i" :node="c" />
    </div>
  </div>
</template>
