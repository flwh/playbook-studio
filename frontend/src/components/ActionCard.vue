<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Icon from "./Icon.vue";
import {
  state,
  updateField,
  removeField,
  removeAction,
  duplicateAction,
  moveAction,
  actionSummary,
  tagClass,
  fieldSpec,
  specOf,
  notify,
} from "../store";
import type { Action, Field } from "../../bindings/playbookstudio/internal/playbook/models";
import { highlight } from "../highlight";

const props = defineProps<{ action: Action; index: number; total: number }>();
const emit = defineEmits<{ (e: "select", i: number): void }>();

const expanded = ref(false);
const editingKey = ref("");
const draft = ref("");
const addKey = ref("");
const addValue = ref("");

const fields = computed(() => props.action.fields ?? []);
const note = computed(() => (props.action.note ?? "").trim());
const selected = computed(() => state.selected === props.index);

function labelOf(f: Field): string {
  return fieldSpec(props.action.tag, f.key)?.label || f.key;
}
function required(f: Field): boolean {
  return !!fieldSpec(props.action.tag, f.key)?.required;
}
function kindOf(f: Field): string {
  const s = fieldSpec(props.action.tag, f.key);
  if (s?.kind) return s.kind;
  if (f.kind === "block") return "textarea";
  if (f.kind === "list") return "list";
  return "text";
}
function enumOf(f: Field): string[] {
  return fieldSpec(props.action.tag, f.key)?.enum ?? [];
}
function valueText(f: Field): string {
  if (f.kind === "list") return (f.items ?? []).join(", ");
  return f.value;
}

const missingRequired = computed(() =>
  (specOf(props.action.tag)?.fields ?? []).filter(
    (s) => s.required && !fields.value.some((f) => f.key === s.key),
  ),
);

const highlightedRaw = computed(() => highlight(props.action.raw ?? "", "yaml"));

const fieldChoices = computed(() => {
  const used = new Set(fields.value.map((f) => f.key));
  return [...(specOf(props.action.tag)?.fields ?? []), ...(state.schema?.common ?? [])].filter(
    (s) => !used.has(s.key),
  );
});

watch(
  () => state.editing,
  (v) => {
    if (v === props.index) expanded.value = true;
    else if (v === -1) expanded.value = false;
  },
);

function toggle() {
  emit("select", props.index);
  expanded.value = !expanded.value;
  state.editing = expanded.value ? props.index : -1;
  if (!expanded.value) editingKey.value = "";
}

function startEdit(f: Field) {
  editingKey.value = f.key;
  draft.value = f.kind === "list" ? (f.items ?? []).join(", ") : f.value;
}
function cancelEdit() {
  editingKey.value = "";
}
async function commitEdit(f: Field) {
  await updateField(props.index, f.key, draft.value);
  editingKey.value = "";
}
async function delField(f: Field) {
  if (!window.confirm(`删除字段「${f.key}」？`)) return;
  await removeField(props.index, f.key);
}
async function quickAdd(key: string) {
  await updateField(props.index, key, "");
  editingKey.value = key;
  draft.value = "";
}
async function addCustom() {
  const k = addKey.value.trim();
  if (!k) {
    notify("字段名不能为空", "err");
    return;
  }
  await updateField(props.index, k, addValue.value);
  addKey.value = "";
  addValue.value = "";
}
function onInput(e: Event) {
  draft.value = (e.target as HTMLInputElement | HTMLTextAreaElement).value;
}
function onSelect(e: Event) {
  draft.value = (e.target as HTMLSelectElement).value;
}
</script>

<template>
  <div class="action-card" :class="{ selected, editing: expanded }">
    <div class="action-row" @click="toggle">
      <span class="idx">{{ index + 1 }}</span>
      <span class="badge tag" :class="tagClass(action.tag)">!{{ action.tag }}</span>
      <span class="summary">{{ actionSummary(action) }}</span>
      <span v-if="action.option" class="badge" :class="action.option.startsWith('!') ? 'neg' : 'cond'">
        {{ action.option }}
      </span>
      <span v-if="action.options && action.options.length" class="badge cond">
        {{ action.options.join(" & ") }}
      </span>
      <span v-if="!action.option && !(action.options && action.options.length)" class="badge cond-none">无条件</span>
      <span v-if="action.weight" class="badge cond-none">w{{ action.weight }}</span>
      <span class="ops" @click.stop>
        <button class="ghost icon" title="上移" :disabled="index === 0" @click="moveAction(index, index - 1)">
          <Icon name="up" :size="13" />
        </button>
        <button class="ghost icon" title="下移" :disabled="index >= total - 1" @click="moveAction(index, index + 1)">
          <Icon name="down" :size="13" />
        </button>
        <button class="ghost icon" title="复制" @click="duplicateAction(index)">
          <Icon name="copy" :size="13" />
        </button>
        <button class="ghost icon" title="删除" @click="removeAction(index)">
          <Icon name="trash" :size="13" />
        </button>
      </span>
    </div>

    <div class="note-line" v-if="note && !expanded" :title="note"># {{ note }}</div>

    <div class="fields" v-if="expanded">
      <div v-if="missingRequired.length" class="alert warn" style="margin: 4px 0 8px">
        <Icon name="alert" :size="15" />
        <div>
          缺少必填字段：
          <template v-for="(m, i) in missingRequired" :key="m.key">
            <button class="ghost" style="padding: 1px 6px" @click="quickAdd(m.key)">+ {{ m.label || m.key }}</button>
            <span v-if="i < missingRequired.length - 1">&nbsp;</span>
          </template>
        </div>
      </div>

      <div class="field-row" v-for="f in fields" :key="f.key">
        <div class="k" :title="f.key">
          {{ labelOf(f) }}<span class="req" v-if="required(f)"> *</span>
        </div>
        <div class="v">
          <template v-if="editingKey === f.key">
            <textarea
              v-if="kindOf(f) === 'textarea'"
              :value="draft"
              @input="onInput"
              @keydown.ctrl.enter="commitEdit(f)"
            ></textarea>
            <select v-else-if="kindOf(f) === 'bool'" :value="draft" @change="onSelect">
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
            <select v-else-if="kindOf(f) === 'enum'" :value="draft" @change="onSelect">
              <option v-if="draft && !enumOf(f).includes(draft)" :value="draft">{{ draft }}（自定义）</option>
              <option v-for="o in enumOf(f)" :key="o" :value="o">{{ o }}</option>
            </select>
            <input
              v-else-if="kindOf(f) === 'int'"
              type="number"
              :value="draft"
              @input="onInput"
              @keydown.enter="commitEdit(f)"
            />
            <input
              v-else
              :value="draft"
              :placeholder="fieldSpec(action.tag, f.key)?.placeholder || ''"
              @input="onInput"
              @keydown.enter="commitEdit(f)"
              @keydown.esc="cancelEdit"
            />
          </template>
          <div v-else class="field-value" :class="{ block: f.kind === 'block' }">{{ valueText(f) }}</div>
        </div>
        <div class="a">
          <template v-if="editingKey === f.key">
            <button class="ghost icon" title="确定" @click="commitEdit(f)"><Icon name="check" :size="13" /></button>
            <button class="ghost icon" title="取消" @click="cancelEdit"><Icon name="close" :size="13" /></button>
          </template>
          <template v-else>
            <button class="ghost icon" title="编辑" @click="startEdit(f)"><Icon name="pencil" :size="13" /></button>
            <button class="ghost icon" title="删除字段" @click="delField(f)"><Icon name="trash" :size="13" /></button>
          </template>
        </div>
      </div>

      <div class="group-title" v-if="fieldChoices.length">添加字段</div>
      <div class="palette-items" v-if="fieldChoices.length">
        <button v-for="c in fieldChoices" :key="c.key" @click="quickAdd(c.key)">+ {{ c.label || c.key }}</button>
      </div>

      <div class="inline-form">
        <input v-model="addKey" placeholder="自定义字段名" />
        <input v-model="addValue" placeholder="值" />
        <button @click="addCustom"><Icon name="plus" :size="13" /> 添加</button>
      </div>

      <div class="group-title">源码片段（高亮）</div>
      <pre class="raw-snippet" v-html="highlightedRaw"></pre>
    </div>
  </div>
</template>
