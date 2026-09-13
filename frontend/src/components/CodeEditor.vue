<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { highlight, langOf, type Lang } from "../highlight";

const props = defineProps<{
  modelValue: string;
  path?: string;
  language?: Lang;
  placeholder?: string;
  minLines?: number;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", v: string): void;
  (e: "apply"): void;
}>();

const ta = ref<HTMLTextAreaElement | null>(null);
const pre = ref<HTMLPreElement | null>(null);
const gut = ref<HTMLPreElement | null>(null);

const lang = computed<Lang>(() => props.language ?? (props.path ? langOf(props.path) : "text"));
const html = computed(() => highlight(props.modelValue ?? "", lang.value));
const gutter = computed(() => {
  const n = (props.modelValue ?? "").split("\n").length;
  let out = "";
  for (let i = 1; i <= n; i++) out += i + "\n";
  return out;
});

function sync() {
  if (!ta.value) return;
  if (pre.value) {
    pre.value.scrollTop = ta.value.scrollTop;
    pre.value.scrollLeft = ta.value.scrollLeft;
  }
  if (gut.value) {
    gut.value.scrollTop = ta.value.scrollTop;
  }
}

function onInput(e: Event) {
  emit("update:modelValue", (e.target as HTMLTextAreaElement).value);
  void nextTick(sync);
}

function onKeydown(e: KeyboardEvent) {
  if (e.ctrlKey && e.key === "Enter") {
    e.preventDefault();
    emit("apply");
    return;
  }
  // Tab 插入两个空格
  if (e.key === "Tab") {
    e.preventDefault();
    const el = e.target as HTMLTextAreaElement;
    const s = el.selectionStart;
    const t = el.selectionEnd;
    const v = el.value.slice(0, s) + "  " + el.value.slice(t);
    emit("update:modelValue", v);
    void nextTick(() => {
      el.selectionStart = el.selectionEnd = s + 2;
      sync();
    });
  }
}

watch(
  () => props.modelValue,
  () => void nextTick(sync),
);

onMounted(() => void nextTick(sync));
</script>

<template>
  <div class="code-wrap" :style="minLines ? { minHeight: minLines * 1.6 * 12 + 24 + 'px' } : undefined">
    <pre class="code-gutter mono" aria-hidden="true" ref="gut">{{ gutter }}</pre>
    <pre class="code-hl" aria-hidden="true" ref="pre"><code v-html="html"></code></pre>
    <textarea
      class="code-input"
      ref="ta"
      :value="modelValue"
      :placeholder="placeholder"
      spellcheck="false"
      wrap="off"
      @input="onInput"
      @scroll="sync"
      @keydown="onKeydown"
    ></textarea>
  </div>
</template>
