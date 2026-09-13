<script setup lang="ts">
import { ref } from "vue";
import Icon from "./Icon.vue";
import { state, pickArchive, openArchive, notify, refreshStatus } from "../store";
import { PlaybookService as PB, errText } from "../api";

const archive = ref(state.status.lastOpenDir ? "" : "");
const password = ref("");
const sevenZip = ref("");
const showZip = ref(false);

async function browse() {
  const p = await pickArchive();
  if (p) archive.value = p;
}

async function open() {
  if (!archive.value) {
    notify("请选择或输入 .apbx 文件路径", "err");
    return;
  }
  await openArchive(archive.value, password.value);
}

async function applySevenZip() {
  try {
    await PB.SetSevenZipPath(sevenZip.value.trim());
    await refreshStatus();
    notify("7z 路径已更新", "ok");
    showZip.value = false;
  } catch (e) {
    notify(errText(e), "err");
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === "Enter") open();
}
</script>

<template>
  <div class="welcome">
    <div class="welcome-card">
      <div class="logo-lg">PB</div>
      <h1>Playbook Studio</h1>
      <p class="sub">
        编辑 AME Playbook（<code>.apbx</code>）中的工作流脚本：解包 → 结构化编辑 YAML 动作 → 重新加密打包。
        所有修改都以最小 diff 写回原文件，注释与格式保持原样。
      </p>

      <div v-if="!state.status.sevenZipOk" class="alert warn">
        <Icon name="alert" :size="16" />
        <div>
          内置 7z 引擎释放失败（可能是缓存目录不可写），且未找到系统 7-Zip。<br />
          可手动指定一个 7z.exe 路径继续使用。
          <div style="margin-top: 8px">
            <button class="ghost" @click="showZip = !showZip">手动指定 7z 路径</button>
          </div>
          <div v-if="showZip" class="file-pick" style="margin-top: 8px">
            <input v-model="sevenZip" placeholder="C:\Program Files\7-Zip\7z.exe" />
            <button class="primary" @click="applySevenZip">使用</button>
          </div>
        </div>
      </div>

      <div class="field">
        <label>Playbook 归档</label>
        <div class="file-pick">
          <input v-model="archive" placeholder="K:\vibecode\Revi-PB-26.04.apbx" @keydown="onKey" />
          <button @click="browse"><Icon name="folder" /> 浏览…</button>
        </div>
      </div>

      <div class="field">
        <label>解包密码</label>
        <input v-model="password" type="password" placeholder="例如 malte" @keydown="onKey" />
        <div class="hint">密码由 playbook 发布方提供；仅保存在本次会话内存中，不会写入磁盘。</div>
      </div>

      <button class="primary" style="width: 100%; justify-content: center; padding: 9px" :disabled="state.busy" @click="open">
        <Icon name="layers" :size="16" />
        {{ state.busy ? "正在解包…" : "打开并解析" }}
      </button>

      <div class="hint" style="margin-top: 14px; text-align: center">
        {{
          state.status.sevenZipSource === "builtin"
            ? "7z 引擎：已内置（无需安装 7-Zip）"
            : "7z: " + (state.status.sevenZip || "未检测到")
        }}
      </div>
    </div>
  </div>
</template>
