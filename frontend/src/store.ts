import { reactive } from "vue";
import { PlaybookService as PB, call, errText } from "./api";
import type { BundleDTO, SchemaDTO, StatusDTO } from "../bindings/playbookstudio/models";
import type {
  Task,
  Issue,
  Action,
  TagSpec,
  FieldSpec,
} from "../bindings/playbookstudio/internal/playbook/models";

export type ToastKind = "info" | "ok" | "err";

export const state = reactive({
  ready: false,
  status: {
    sevenZip: "",
    sevenZipOk: false,
    hasBundle: false,
    archive: "",
    dirty: false,
    lastOpenDir: "",
  } as StatusDTO,
  schema: null as SchemaDTO | null,
  bundle: null as BundleDTO | null,
  task: null as Task | null,
  currentPath: "",
  selected: -1,
  editing: -1,
  issues: [] as Issue[],
  password: "",
  busy: false,
  validating: false,
  toast: "",
  toastKind: "info" as ToastKind,
  rightTab: "validate" as "validate" | "conf",
  centerTab: "actions" as "actions" | "source",
  leftTab: "tree" as "tree" | "files",
  showPalette: false,
  filePath: "",
  fileContent: "",
  fileDirty: false,
});

let toastTimer = 0;

export function notify(msg: string, kind: ToastKind = "info") {
  state.toast = msg;
  state.toastKind = kind;
  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    if (state.toast === msg) state.toast = "";
  }, 4000);
}

export function fail(e: unknown, prefix = "") {
  const msg = errText(e);
  notify(prefix ? `${prefix}: ${msg}` : msg, "err");
  console.error(e);
}

/* ---------- schema 辅助 ---------- */

export function specOf(tag: string): TagSpec | undefined {
  const tags = state.schema?.tags ?? [];
  return tags.find((t) => t.tag === tag);
}

export function fieldSpec(tag: string, key: string): FieldSpec | undefined {
  const s = specOf(tag);
  if (s?.fields) {
    const f = s.fields.find((x) => x.key === key);
    if (f) return f;
  }
  return (state.schema?.common ?? []).find((x) => x.key === key);
}

export function tagTitle(tag: string): string {
  return specOf(tag)?.title ?? tag;
}

export function tagClass(tag: string): string {
  const g = specOf(tag)?.group ?? "";
  switch (g) {
    case "流程":
      return "t-flow";
    case "注册表":
      return "t-reg";
    case "执行":
      return "t-exec";
    case "系统":
      return "t-sys";
    default:
      return "";
  }
}

/** 字段的渲染类型（int/bool/list 需要按类型写回，避免写出 'true' 这类字符串） */
export function fieldRenderKind(tag: string, key: string): string {
  switch (key) {
    case "weight":
    case "startup":
      return "int";
    case "exeDir":
    case "wait":
    case "ignoreErrors":
    case "overwrite":
    case "showOutput":
    case "showError":
      return "bool";
    case "options":
      return "list";
  }
  const f = fieldSpec(tag, key);
  if (f) {
    if (f.kind === "int") return "int";
    if (f.kind === "bool") return "bool";
    if (f.kind === "list") return "list";
  }
  return "auto";
}

export function actionSummary(a: Action): string {
  const v = (k: string) => {
    const f = (a.fields ?? []).find((x) => x.key === k);
    if (!f) return "";
    if (f.kind === "list") return (f.items ?? []).join(",");
    return f.value;
  };
  const tag = a.tag;
  const parts: string[] = [];
  switch (tag) {
    case "task":
      parts.push(v("path"));
      break;
    case "registryValue":
      parts.push(v("path"));
      if (v("operation") === "delete") parts.push("删除");
      if (v("value")) parts.push(v("value"));
      if (v("data")) parts.push("= " + v("data"));
      break;
    case "registryKey":
      parts.push(v("path"), v("operation") || "add");
      break;
    case "run":
      parts.push([v("exe"), v("args")].filter(Boolean).join(" "));
      break;
    case "cmd":
    case "powerShell":
      parts.push(v("command").replace(/\s+/g, " ").slice(0, 140));
      break;
    case "writeStatus":
    case "status":
      parts.push(v("status"));
      break;
    case "taskKill":
      parts.push(v("name"));
      break;
    case "download":
      parts.push(v("destination"), "<- " + v("url"));
      break;
    case "service":
      parts.push(v("name"), "startup=" + v("startup"));
      break;
    case "appx":
      parts.push(v("operation"), v("name"));
      break;
    case "file":
      parts.push(v("path"));
      break;
    case "software":
      parts.push(v("name"), v("source"), v("package"));
      break;
    default:
      parts.push(
        (a.fields ?? [])
          .map((f) => `${f.key}=${f.kind === "list" ? (f.items ?? []).join(",") : f.value}`)
          .slice(0, 3)
          .join(" "),
      );
  }
  return parts.filter((x) => x && x.trim() !== "").join("  ");
}

/* ---------- 生命周期 ---------- */

export async function initApp() {
  try {
    state.schema = await call(PB.Schema());
  } catch (e) {
    fail(e, "读取 schema 失败");
  }
  await refreshStatus();
  state.ready = true;
  try {
    const s = await PB.StartupFile();
    if (s && s.archive) {
      await openArchive(s.archive, s.password ?? "");
    }
  } catch {
    /* 无启动参数 */
  }
}

export async function refreshStatus() {
  try {
    state.status = await call(PB.Status());
  } catch (e) {
    fail(e, "读取状态失败");
  }
}

/* ---------- 打开 / 关闭 ---------- */

export async function pickArchive(): Promise<string> {
  try {
    return await call(PB.PickArchive());
  } catch (e) {
    fail(e);
    return "";
  }
}

export async function openArchive(path: string, password: string) {
  if (!path) {
    notify("请先选择 .apbx 文件", "err");
    return;
  }
  state.busy = true;
  try {
    const b = await call(PB.OpenBundle(path, password));
    state.bundle = b;
    state.password = password;
    state.selected = -1;
    state.editing = -1;
    state.task = null;
    state.currentPath = "";
    state.issues = [];
    const main = b.main || (b.taskPaths ?? [])[0] || "";
    if (main) await selectTask(main);
    notify(`已载入 ${b.summary?.title || b.archive}`, "ok");
    await runValidate();
  } catch (e) {
    fail(e, "打开失败");
  } finally {
    state.busy = false;
    await refreshStatus();
  }
}

export async function closeBundle() {
  try {
    await PB.CloseBundle();
  } catch (e) {
    fail(e);
  }
  state.bundle = null;
  state.task = null;
  state.currentPath = "";
  state.issues = [];
  await refreshStatus();
}

export async function reloadBundle() {
  if (isDirty() && !window.confirm("存在未保存修改，重新载入会丢弃这些修改，确定继续？")) return;
  state.busy = true;
  try {
    const b = await call(PB.Reload());
    state.bundle = b;
    state.selected = -1;
    state.editing = -1;
    if (state.currentPath) await selectTask(state.currentPath);
    notify("已重新载入", "ok");
    await runValidate();
  } catch (e) {
    fail(e, "重新载入失败");
  } finally {
    state.busy = false;
  }
}

export function isDirty(): boolean {
  return (state.bundle?.dirty ?? []).length > 0;
}

/* ---------- 任务 ---------- */

export async function selectTask(path: string) {
  if (!path) return;
  if (state.currentPath === path && state.task) return;
  state.busy = true;
  try {
    state.task = await call(PB.Task(path));
    state.currentPath = path;
    state.filePath = "";
    state.selected = -1;
    state.editing = -1;
    state.centerTab = "actions";
  } catch (e) {
    fail(e, "读取任务失败");
  } finally {
    state.busy = false;
  }
}

/* ---------- 任意文本文件 ---------- */

export async function openRawFile(rel: string) {
  try {
    state.fileContent = await call(PB.ReadFile(rel));
    state.filePath = rel;
    state.currentPath = "";
    state.task = null;
    state.fileDirty = false;
    state.selected = -1;
    state.editing = -1;
  } catch (e) {
    fail(e, "读取文件失败");
  }
}

export async function saveRawFile() {
  if (!state.filePath) return;
  try {
    await PB.SaveFile(state.filePath, state.fileContent);
    state.fileDirty = false;
    markDirtyLocal(state.filePath);
    notify(`已保存 ${state.filePath}`, "ok");
    await refreshBundleLight();
  } catch (e) {
    fail(e, "保存失败");
  }
}

async function refreshBundleLight() {
  try {
    state.bundle = await call(PB.Bundle());
  } catch {
    /* 忽略 */
  }
}

const KINDS_OUT_OF_BUNDLE = new Set<string>();

/** 把后端返回的新任务写入 state */
function applyTask(t: Task | null | undefined) {
  if (!t) return;
  state.task = t;
  markDirtyLocal(t.path);
}

function markDirtyLocal(rel: string) {
  if (!state.bundle) return;
  const list = new Set(state.bundle.dirty ?? []);
  list.add(rel);
  state.bundle.dirty = [...list];
  state.status.dirty = true;
}

export async function updateField(index: number, key: string, value: string, kind?: string) {
  if (!state.task) return;
  const tag = state.task.actions?.[index]?.tag ?? "";
  const k = kind || fieldRenderKind(tag, key);
  try {
    applyTask(await PB.UpdateField(state.task.path, index, key, value, k));
  } catch (e) {
    fail(e, "写入失败");
  }
}

export async function removeField(index: number, key: string) {
  if (!state.task) return;
  try {
    applyTask(await PB.RemoveField(state.task.path, index, key));
  } catch (e) {
    fail(e, "删除字段失败");
  }
}

export async function addAction(index: number, tag: string, fields: { key: string; value: string; kind: string }[], flow: boolean) {
  if (!state.task) return;
  try {
    applyTask(await PB.AddAction(state.task.path, index, tag, fields, flow));
    state.selected = index;
    state.editing = index;
    await refreshBundleLight();
    notify(`已插入 !${tag}`, "ok");
  } catch (e) {
    fail(e, "新增失败");
  }
}

export async function removeAction(index: number) {
  if (!state.task) return;
  const a = state.task.actions?.[index];
  if (!window.confirm(`确定删除第 ${index + 1} 个动作 (!${a?.tag}) 吗？`)) return;
  try {
    applyTask(await PB.RemoveAction(state.task.path, index));
    state.selected = -1;
    state.editing = -1;
    await refreshBundleLight();
    notify("已删除", "ok");
  } catch (e) {
    fail(e, "删除失败");
  }
}

export async function duplicateAction(index: number) {
  if (!state.task) return;
  try {
    applyTask(await PB.DuplicateAction(state.task.path, index));
    await refreshBundleLight();
    notify("已复制", "ok");
  } catch (e) {
    fail(e, "复制失败");
  }
}

export async function moveAction(from: number, to: number) {
  if (!state.task) return;
  if (to < 0) return;
  try {
    applyTask(await PB.MoveAction(state.task.path, from, to));
    state.selected = to;
    await refreshBundleLight();
  } catch (e) {
    fail(e, "移动失败");
  }
}

export async function setMeta(key: string, value: string) {
  if (!state.task) return;
  try {
    applyTask(await PB.SetMeta(state.task.path, key, value));
  } catch (e) {
    fail(e, "写入失败");
  }
}

export async function applySource(text: string) {
  if (!state.task) return;
  try {
    applyTask(await PB.SetTaskText(state.task.path, text));
    await refreshBundleLight();
    notify("源码已应用", "ok");
  } catch (e) {
    fail(e, "YAML 解析失败");
  }
}

export async function discardSource() {
  if (!state.task) return;
  const p = state.task.path;
  state.task = null;
  await selectTask(p);
}

/* ---------- conf ---------- */

export async function setConfText(tag: string, occ: number, value: string) {
  try {
    const c = await call(PB.SetConfText(tag, occ, value));
    if (state.bundle) {
      state.bundle.conf = c;
      state.bundle.summary = c.summary;
    }
    markDirtyLocal("playbook.conf");
  } catch (e) {
    fail(e, "写入 conf 失败");
  }
}

export async function setConfAttr(tag: string, occ: number, attr: string, value: string) {
  try {
    const c = await call(PB.SetConfAttr(tag, occ, attr, value));
    if (state.bundle) state.bundle.conf = c;
    markDirtyLocal("playbook.conf");
  } catch (e) {
    fail(e, "写入 conf 失败");
  }
}

export async function setConfRaw(text: string) {
  try {
    const c = await call(PB.SetConfRaw(text));
    if (state.bundle) {
      state.bundle.conf = c;
      state.bundle.summary = c.summary;
    }
    markDirtyLocal("playbook.conf");
    notify("playbook.conf 已应用", "ok");
  } catch (e) {
    fail(e, "写入 conf 失败");
  }
}

/* ---------- 校验 / 保存 ---------- */

export async function runValidate() {
  if (!state.bundle) return;
  state.validating = true;
  try {
    state.issues = (await PB.Validate()) ?? [];
    state.bundle.dirty = state.bundle.dirty ?? [];
  } catch (e) {
    fail(e, "校验失败");
  } finally {
    state.validating = false;
    state.rightTab = "validate";
  }
}

export async function saveBundle(saveAs: boolean) {
  if (!state.bundle) return;
  const archive = state.bundle.archive;
  const base = archive.split(/[\\/]/).pop() || "playbook.apbx";
  let out = "";
  if (saveAs) {
    out = await pickSavePath(base);
    if (!out) return;
  }
  state.busy = true;
  try {
    const r = await call(PB.Save(out));
    notify(`已保存到 ${r.archive}（${(r.size / 1024).toFixed(0)} KB，${r.files} 个文件）`, "ok");
    await refreshStatus();
    await refreshBundleLight();
  } catch (e) {
    fail(e, "保存失败");
  } finally {
    state.busy = false;
  }
}

export async function pickSavePath(defaultName: string): Promise<string> {
  try {
    return await call(PB.PickSavePath(defaultName));
  } catch (e) {
    fail(e);
    return "";
  }
}

export async function reveal(p: string) {
  try {
    await PB.RevealInExplorer(p);
  } catch (e) {
    fail(e);
  }
}

export async function exportWorkspace(dir: string) {
  try {
    await PB.ExportWorkspace(dir);
    notify(`已导出到 ${dir}`, "ok");
  } catch (e) {
    fail(e, "导出失败");
  }
}

void KINDS_OUT_OF_BUNDLE;
