# Playbook Studio

[![Build Windows](https://github.com/flwh/playbook-studio/actions/workflows/build-windows.yml/badge.svg)](https://github.com/flwh/playbook-studio/actions/workflows/build-windows.yml)

AME Playbook（`.apbx`）工作流脚本编辑器 —— 基于 **Wails v3 + Vue 3** 的 Windows 桌面应用，用于打开、编辑并重新打包加密的 `.apbx` 归档。

## 背景

`.apbx` 是 AME Playbook（ReviOS 等系统定制方案使用）的打包格式：

- 本质是一个 **7z 归档**，采用 AES-256 加密，且**加密了文件名**（`-mhe=on`）—— 没有密码时连目录结构都看不到
- 归档内是 XML 元数据 `playbook.conf` + `Configuration/Tasks/**/*.yml` 形式的工作流脚本
- 这些 YAML 使用自定义标签 DSL 描述动作：

```yaml
- !registryValue:
    path: 'HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced'
    value: 'TaskbarSmallIcons'
    type: REG_DWORD
    data: '1'
    option: '!HideTaskbarIcons'   # 以 ! 开头表示取反
```

用通用 YAML 编辑器修改这类文件会**丢失注释、重排缩进、改变引号风格**，产生大量无意义的 diff，甚至改变语义（`true` 与 `'true'` 在 YAML 里是两回事）。Playbook Studio 就是针对这个格式的专用编辑器。

## 核心特性

### 解锁与浏览

- 打开 AES-256 加密的 `.apbx`；支持从命令行直接带归档路径与密码启动，也注册了 `.apbx` 文件关联
- 按 `!task` 引用关系重建任务树，标出**缺失的引用**与**循环引用**
- 未被主任务引用的孤立任务文件会被标记

### 编辑

- **源码级「手术式」编辑**：修改某个字段时，只替换该字段在源码中占据的那一段文本，其余部分**逐字节不变** —— 注释、空行、缩进、引号风格全部原样保留
- 动作的增、删、改与排序；字段按 schema 渲染为文本框 / 多行 / 整数 / 布尔 / 枚举 / 列表
- 任务树、动作列表与 YAML 源码视图联动，可定位到具体行
- `playbook.conf` 元数据编辑（名称、版本、作者、描述、FeaturePages 等）
- 内置 YAML 语法高亮

### 校验

错误 / 警告 / 提示三级，可定位到文件与行号。**默认只显示错误**（警告多为启发式判断，对合法 playbook 也会提示），需要时可在面板中勾选显示。

| 级别 | 示例 |
|---|---|
| 错误 | 引用的任务文件不存在、缺少必填字段、`!cmd`/`!powerShell` 命令为空、`!run` 缺少 exe |
| 警告 | 未知的 action 类型、缺少或未知的 `privilege`、`option` 未在 `playbook.conf` 的 FeaturePages 中定义、`option` 与 `options` 同时使用、`REG_DWORD` 的 data 非数字 |
| 提示 | 任务文件未被主任务引用、删除整个注册表值 |

### 写回

- 保存时把工作区改动重新打包为 7z + AES-256（**保留头加密**，压缩级别 7），可覆盖原文件或另存为新文件
- 也可把解包内容导出到目录，便于用外部工具查看

## 支持的 action 类型

共 16 种，按分组：

| 分组 | 标签 |
|---|---|
| 注册表 | `!registryValue` `!registryKey` |
| 流程 | `!task` `!writeStatus` `!status` `!taskKill` `!scheduledTask` |
| 执行 | `!run` `!cmd` `!powerShell` `!download` |
| 系统 | `!service` `!appx` `!file` `!software` `!systemPackage`（已废弃） |

所有动作都支持通用字段：`option`（条件）、`options`（条件组）、`weight`（权重，越大越晚执行）、`errorAction`（`Stop` / `Continue` / `Ignore`）。

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面框架 | [Wails v3](https://v3.wails.io/) `v3.0.0-beta.20` |
| 后端 | Go 1.25 |
| 前端 | Vue 3 + TypeScript + Vite 8 |
| YAML | `gopkg.in/yaml.v3`（保留注释与节点位置信息） |
| 归档 | 内嵌 7-Zip 独立控制台版 `7zr.exe` |

## 目录结构

```
main.go                 应用入口、窗口与启动参数（归档路径 / 密码）
service.go              暴露给前端的服务：打开 / 保存 / 校验 / 打包 / 导出
applog.go               日志
internal/playbook/      playbook 解析与编辑引擎
  ├─ task.go            任务与动作模型、源码区间计算
  ├─ edit.go            字段与动作的源码级编辑操作
  ├─ scalar.go          标量渲染（引号风格、块标量、列表）
  ├─ conf.go            playbook.conf 元数据读写
  ├─ bundle.go          归档工作区管理、保存回写
  ├─ schema.go          15 种 action 的字段定义
  └─ validate.go        静态校验规则
internal/sevenzip/      7z 封装（内嵌 7zr.exe，首次运行释放到用户缓存目录）
frontend/               Vue 3 界面
.github/workflows/      CI
```

## 开发

需要 Go 1.25+、Node 20.19+ / 22.12+，以及 Wails v3 CLI：

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.20

wails3 dev                    # 开发模式（前后端热重载）
wails3 build                  # 构建当前平台
wails3 task windows:build     # 明确构建 Windows 版本
```

注意：顶层 `build` 任务会按宿主系统分发（`GOOS` 默认是宿主 OS），在 Ubuntu 上直接 `wails3 build` 会构建 Linux 版本，交叉编译时要点名 `windows:build`。

## 构建与发布

`.github/workflows/build-windows.yml` 在 **Ubuntu** 上交叉编译 Windows x64 版本 —— 纯 Go 交叉编译，不需要 Docker / MinGW / NSIS。产物是单文件 `playbook-studio-windows-amd64.exe`，从 Actions 页面的 Artifacts 下载。

触发条件：推送到 `main`、推送 `v*` 标签、PR、或手动启动。

## 运行时依赖

**目标机器无需安装 7-Zip。** 程序内嵌了 7-Zip 官方独立控制台版 `7zr.exe`，首次运行时释放到 `%LOCALAPPDATA%\PlaybookStudio\tools\`。查找顺序：

1. 环境变量 `PLAYBOOK_STUDIO_7Z`
2. 内置版本（默认）
3. 程序同目录的 `7z.exe` 或 `tools\7z.exe`
4. 系统安装的 7-Zip（含 Microsoft Store 版）

> 补充：7-Zip 官方只发布 x86 版的 `7zr.exe`，它在 64 位 Windows 上原生运行，无需额外处理。

## 第三方许可

- **7-Zip**：LGPL。内嵌二进制与许可证见 `internal/sevenzip/bin/`（`LICENSE-7zip.txt`）
- **Inter 字体**：SIL Open Font License，见 `frontend/Inter Font License.txt`

## 已知限制

- 构建产物目前只提供 **Windows x64**
- 校验规则是启发式的：`option` 未在 FeaturePages 中定义这类情况可能是刻意设计，因此归为警告而不是错误
- 打包参数（头加密、压缩级别）在代码中固定，尚未开放到界面
