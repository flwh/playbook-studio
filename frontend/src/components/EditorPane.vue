<script setup lang="ts">
import { computed, ref, watch, nextTick } from "vue";
import Icon from "./Icon.vue";
import ActionCard from "./ActionCard.vue";
import CodeEditor from "./CodeEditor.vue";
import { state, addAction, setMeta, applySource, saveRawFile, reveal, tagClass, specOf } from "../store";
import type { KeyValue } from "../../bindings/playbookstudio/internal/playbook/models";

const sourceDraft = ref("");
const fileDraft = ref("");

watch(
  () => [state.task?.path, state.centerTab],
  () => {
    if (state.centerTab === "source" && state.task) sourceDraft.value = state.task.text;
  },
  { immediate: true },
);

watch(
  () => state.filePath,
  () => {
    fileDraft.value = state.fileContent;
  },
  { immediate: true },
);

const actions = computed(() => state.task?.actions ?? []);
const insertAt = computed(() => (state.selected >= 0 ? state.selected + 1 : actions.value.length));

const groupedTags = computed(() => {
  const groups = state.schema?.groups ?? [];
  const tags = state.schema?.tags ?? [];
  return groups.map((g) => ({ group: g, items: tags.filter((t) => t.group === g) }));
});

function onSelect(i: number) {
  state.selected = i;
}

function defaultFields(tag: string): KeyValue[] {
  const spec = specOf(tag);
  const out: KeyValue[] = [];
  for (const f of spec?.fields ?? []) {
    if (f.required) out.push({ key: f.key, value: "", kind: "" });
  }
  return out;
}

async function pickTag(tag: string) {
  const spec = specOf(tag);
  const at = insertAt.value;
  state.showPalette = false;
  await addAction(at, tag, defaultFields(tag), !!spec?.flow);
  await nextTick();
  document.getElementById("action-" + at)?.scrollIntoView({ block: "center", behavior: "smooth" });
}

async function apply() {
  if (!state.task) return;
  await applySource(sourceDraft.value);
  sourceDraft.value = state.task.text;
}

function onFileEdit(v: string) {
  fileDraft.value = v;
  state.fileDirty = true;
}

function resetSource() {
  if (state.task) sourceDraft.value = state.task.text;
}
</script>

<template>
  <!-- 未选择任何内容 -->
  <div class="panel" v-if="!state.task && !state.filePath">
    <div class="empty">
      <div>
        <div class="big">◍</div>
        从左侧任务树选择一个任务开始编辑<br />
        <span style="font-size: 12px">或在「文件」标签里打开 Executables 下的脚本</span>
      </div>
    </div>
  </div>

  <!-- 任意文本文件 -->
  <div class="panel" v-else-if="state.filePath">
    <div class="panel-head">
      <Icon name="terminal" :size="14" />
      <span class="mono" style="font-size: 12px; text-transform: none">{{ state.filePath }}</span>
      <div style="flex: 1"></div>
      <button class="ghost" @click="reveal(state.filePath)"><Icon name="external" :size="13" /> 定位</button>
      <button class="primary" :disabled="!state.fileDirty" @click="saveRawFile">
        <Icon name="save" :size="13" /> 保存文件
      </button>
    </div>
    <CodeEditor :model-value="fileDraft" :path="state.filePath" @update:model-value="onFileEdit" />
  </div>

  <!-- 任务编辑 -->
  <div class="panel" v-else-if="state.task">
    <div class="task-head">
      <h2>{{ state.task.meta.title || "(未命名任务)" }}</h2>
      <div class="path">{{ state.task.path }}</div>
      <div class="row">
        <div class="meta-field">
          <span>标题</span>
          <input :value="state.task.meta.title" @change="setMeta('title', ($event.target as HTMLInputElement).value)" />
        </div>
        <div class="meta-field">
          <span>权限</span>
          <select
            :value="state.task.meta.privilege"
            @change="setMeta('privilege', ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="p in state.schema?.privileges ?? []" :key="p" :value="p">{{ p }}</option>
            <option v-if="state.task.meta.privilege && !(state.schema?.privileges ?? []).includes(state.task.meta.privilege)" :value="state.task.meta.privilege">
              {{ state.task.meta.privilege }}
            </option>
          </select>
        </div>
        <div class="meta-field">
          <span>描述</span>
          <input
            :value="state.task.meta.description"
            @change="setMeta('description', ($event.target as HTMLInputElement).value)"
          />
        </div>
        <span class="badge cond-none">{{ actions.length }} 个动作</span>
      </div>
    </div>

    <div class="panel-head">
      <div class="tabs">
        <button :class="{ active: state.centerTab === 'actions' }" @click="state.centerTab = 'actions'">
          <Icon name="list" :size="13" /> 动作
        </button>
        <button :class="{ active: state.centerTab === 'source' }" @click="state.centerTab = 'source'">
          <Icon name="code" :size="13" /> 源码
        </button>
      </div>
      <div style="flex: 1"></div>
      <button v-if="state.centerTab === 'actions'" @click="state.showPalette = !state.showPalette">
        <Icon name="plus" :size="13" /> 新增动作
      </button>
      <template v-else>
        <button @click="resetSource"><Icon name="refresh" :size="13" /> 重置</button>
        <button class="primary" @click="apply"><Icon name="check" :size="13" /> 应用修改</button>
      </template>
    </div>

    <div class="panel-body flush" v-if="state.centerTab === 'actions'">
      <div class="palette" v-if="state.showPalette">
        <div class="palette-head">
          <Icon name="plus" :size="14" />
          <b>新增动作</b>
          <span style="color: var(--muted)">插入到第 {{ insertAt + 1 }} 项</span>
          <div style="flex: 1"></div>
          <button class="ghost icon" @click="state.showPalette = false"><Icon name="close" :size="13" /></button>
        </div>
        <div class="palette-body">
          <div class="palette-group" v-for="g in groupedTags" :key="g.group">
            <div class="gname">{{ g.group }}</div>
            <div class="palette-items">
              <button v-for="t in g.items" :key="t.tag" :class="tagClass(t.tag)" :title="'!' + t.tag" @click="pickTag(t.tag)">
                {{ t.title }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="actions">
        <ActionCard
          v-for="(a, i) in actions"
          :id="'action-' + i"
          :key="i"
          :action="a"
          :index="i"
          :total="actions.length"
          @select="onSelect"
        />
        <div v-if="!actions.length" class="empty" style="height: 200px">这个任务还没有任何动作</div>
      </div>
    </div>

    <div class="panel-body flush" v-else>
      <div class="source-wrap">
        <CodeEditor v-model="sourceDraft" :path="state.task.path" placeholder="YAML 源码" @apply="apply" />
      </div>
    </div>
  </div>
</template>
