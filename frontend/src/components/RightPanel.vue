<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import Icon from "./Icon.vue";
import CodeEditor from "./CodeEditor.vue";
import {
  state,
  runValidate,
  setConfText,
  setConfAttr,
  setConfRaw,
  selectTask,
  notify,
} from "../store";
import type { Issue } from "../../bindings/playbookstudio/internal/playbook/models";

const confDraft = ref("");
const confSearch = ref("");
const confEditing = ref(false);

const conf = computed(() => state.bundle?.conf ?? null);
const fields = computed(() => conf.value?.fields ?? []);
const attrs = computed(() => conf.value?.attrs ?? []);
const pages = computed(() => conf.value?.pages ?? []);
const packages = computed(() => conf.value?.packages ?? []);

const issueCount = computed(() => ({
  error: state.issues.filter((i) => i.level === "error").length,
  warning: state.issues.filter((i) => i.level === "warning").length,
  info: state.issues.filter((i) => i.level === "info").length,
}));

// 默认只显示错误：警告与提示多为启发式判断，对合法 playbook 也会报，收进开关里
const showTips = ref(false);
const visibleIssues = computed(() =>
  state.issues.filter(
    (i) => i.level === "error" || (showTips.value && (i.level === "warning" || i.level === "info")),
  ),
);
const hiddenCount = computed(() => state.issues.length - visibleIssues.value.length);

const basicFields: [string, string][] = [
  ["Playbook/Name", "名称"],
  ["Playbook/Username", "作者"],
  ["Playbook/Title", "标题"],
  ["Playbook/Version", "版本"],
  ["Playbook/UniqueId", "UniqueId"],
  ["Playbook/ShortDescription", "一句话简介"],
  ["Playbook/Description", "描述"],
  ["Playbook/Details", "详情"],
  ["Playbook/ProductCode", "产品代码"],
  ["Playbook/Git", "Git 仓库"],
  ["Playbook/Website", "网站"],
  ["Playbook/DonateLink", "捐赠链接"],
  ["Playbook/UpgradableFrom", "可从版本升级"],
];

function cf(path: string) {
  return fields.value.find((f) => f.path === path);
}
function valueOf(path: string): string {
  return cf(path)?.value ?? "";
}
function exists(path: string): boolean {
  return !!cf(path);
}
async function setText(path: string, value: string) {
  const f = cf(path);
  if (!f) {
    notify(`找不到配置项 ${path}`, "err");
    return;
  }
  await setConfText(f.tag, f.occurrence, value);
}

async function togglePageRequired(tag: string, occ: number, v: boolean) {
  await setConfAttr(tag, occ, "IsRequired", v ? "true" : "false");
}
async function setDefault(tag: string, occ: number, name: string) {
  await setConfAttr(tag, occ, "DefaultOption", name);
}
async function setChecked(tag: string, occ: number, v: boolean) {
  await setConfAttr(tag, occ, "IsChecked", v ? "true" : "false");
}
async function setName(occ: number, v: string) {
  await setConfText("Name", occ, v);
}
async function setLabel(occ: number, v: string) {
  await setConfText("Text", occ, v);
}

async function gotoIssue(it: Issue) {
  if (it.file === "playbook.conf") {
    state.rightTab = "conf";
    return;
  }
  if (it.file && (state.bundle?.taskPaths ?? []).includes(it.file)) {
    await selectTask(it.file);
    if (it.index >= 0) {
      state.selected = it.index;
      state.editing = it.index;
      await nextTick();
      document.getElementById("action-" + it.index)?.scrollIntoView({ block: "center", behavior: "smooth" });
    }
  } else if (it.file) {
    notify(it.file, "info");
  }
}

const filteredAttrs = computed(() => {
  const q = confSearch.value.trim().toLowerCase();
  if (!q) return [];
  return attrs.value.filter(
    (a) => a.path.toLowerCase().includes(q) || a.name.toLowerCase().includes(q) || a.value.toLowerCase().includes(q),
  ).slice(0, 120);
});

function startConfEdit() {
  confDraft.value = conf.value?.raw ?? "";
  confEditing.value = true;
}
async function applyConfEdit() {
  await setConfRaw(confDraft.value);
  confEditing.value = false;
}

const otherFields = computed(() =>
  fields.value.filter((f) => !f.path.startsWith("Playbook/") || f.path.includes("/")),
);
</script>

<template>
  <div class="panel">
    <div class="panel-head">
      <div class="tabs">
        <button :class="{ active: state.rightTab === 'validate' }" @click="state.rightTab = 'validate'">
          <Icon name="shield" :size="13" /> 校验
          <span v-if="issueCount.error" class="badge" style="padding: 0 5px">{{ issueCount.error }}</span>
        </button>
        <button :class="{ active: state.rightTab === 'conf' }" @click="state.rightTab = 'conf'">
          <Icon name="wrench" :size="13" /> 配置
        </button>
      </div>
      <div style="flex: 1"></div>
      <button v-if="state.rightTab === 'validate'" class="ghost icon" title="重新校验" :disabled="state.validating" @click="runValidate">
        <Icon name="refresh" :size="13" :class="{ spin: state.validating }" />
      </button>
      <button v-else-if="!confEditing" class="ghost icon" title="编辑源码" @click="startConfEdit">
        <Icon name="code" :size="13" />
      </button>
      <template v-else>
        <button class="ghost icon" title="取消" @click="confEditing = false"><Icon name="close" :size="13" /></button>
        <button class="ghost icon" title="应用" @click="applyConfEdit"><Icon name="check" :size="13" /></button>
      </template>
    </div>

    <!-- 校验 -->
    <div class="panel-body" v-if="state.rightTab === 'validate'">
      <div class="issue-bar">
        <span class="badge" style="color: var(--err)">错误 {{ issueCount.error }}</span>
        <template v-if="showTips">
          <span class="badge" style="color: var(--warn)">警告 {{ issueCount.warning }}</span>
          <span class="badge" style="color: var(--info)">提示 {{ issueCount.info }}</span>
        </template>
        <div style="flex: 1"></div>
        <label class="tips-toggle">
          <input type="checkbox" v-model="showTips" />
          显示警告与提示
        </label>
      </div>
      <div v-if="hiddenCount && !showTips" class="tips-hint">
        已隐藏 {{ hiddenCount }} 条启发式警告/提示（多为误报），需要时勾选上方开关查看。
      </div>
      <div v-if="!visibleIssues.length" class="empty" style="height: 160px">没有发现问题 ✓</div>
      <div class="issue" v-for="(it, i) in visibleIssues" :key="i" :class="it.level" @click="gotoIssue(it)">
        <span class="ic">
          <Icon :name="it.level === 'error' ? 'xcircle' : it.level === 'warning' ? 'alert' : 'info'" :size="15" />
        </span>
        <span class="msg">
          {{ it.message }}
          <div class="loc">{{ it.file }}<template v-if="it.line">:{{ it.line }}</template> · !{{ it.tag }}</div>
        </span>
      </div>
    </div>

    <!-- 配置 -->
    <div class="panel-body" v-else>
      <div v-if="confEditing" style="height: calc(100vh - 160px); display: flex">
        <CodeEditor v-model="confDraft" language="xml" />
      </div>
      <template v-else-if="!conf">
        <div class="empty">归档里没有 playbook.conf</div>
      </template>
      <template v-else>
        <div class="conf-section">
          <h4>基本信息</h4>
          <div class="conf-grid">
            <div class="conf-item" v-for="[path, label] in basicFields" :key="path">
              <label :title="path">{{ label }}</label>
              <input
                v-if="exists(path)"
                :value="valueOf(path)"
                @change="setText(path, ($event.target as HTMLInputElement).value)"
              />
              <span v-else class="mono" style="color: var(--muted); font-size: 11px">未定义</span>
            </div>
          </div>
        </div>

        <div class="conf-section" v-for="(p, pi) in pages" :key="pi">
          <h4>
            选项页 {{ pi + 1 }} · {{ p.kind }}
            <span class="badge cond-none" style="margin-left: 6px">{{ (p.options ?? []).length }} 项</span>
          </h4>
          <div class="conf-item wide" v-if="p.description" style="margin-bottom: 6px">
            <label style="font-size: 11px">{{ p.description }}</label>
          </div>
          <div class="conf-opt" v-for="o in p.options ?? []" :key="o.tag + o.occurrence">
            <input
              v-if="o.hasChecked"
              type="checkbox"
              :checked="o.isChecked"
              @change="setChecked(o.tag, o.occurrence, ($event.target as HTMLInputElement).checked)"
            />
            <input
              v-else
              type="radio"
              :name="'page-' + pi"
              :checked="p.defaultOption === o.name"
              @change="setDefault(p.tag, p.occurrence, o.name)"
            />
            <input style="max-width: 190px" :value="o.text" @change="setLabel(o.textOccurrence, ($event.target as HTMLInputElement).value)" />
            <input
              class="mono"
              style="max-width: 210px"
              :value="o.name"
              :disabled="o.isNone"
              @change="setName(o.nameOccurrence, ($event.target as HTMLInputElement).value)"
            />
            <span v-if="o.isNone" class="badge cond-none">none</span>
          </div>
        </div>

        <div class="conf-section" v-if="packages.length">
          <h4>软件包</h4>
          <div class="conf-opt" style="grid-template-columns: 1fr auto" v-for="(pk, i) in packages" :key="i">
            <div>
              <b>{{ pk.title || pk.name }}</b>
              <div class="mono" style="color: var(--muted); font-size: 11px">{{ pk.option }} · {{ pk.name }}</div>
            </div>
          </div>
        </div>

        <div class="conf-section">
          <h4>其余配置项（{{ otherFields.length }}）</h4>
          <input v-model="confSearch" placeholder="搜索属性名 / 路径 / 值（只读预览）" />
          <div style="margin-top: 8px">
            <div class="conf-item" v-for="(a, i) in filteredAttrs" :key="i">
              <label :title="a.path">{{ a.path }}<span style="color: var(--muted)"> @{{ a.name }}</span></label>
              <span class="mono" style="font-size: 11px; word-break: break-all">{{ a.value }}</span>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
