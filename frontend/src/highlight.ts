// 轻量语法高亮：只做词法着色，输出与原文逐字符对齐（便于与 textarea 叠加渲染）
// 支持 yaml / xml / ps1(cmd) / json / text

export function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function span(cls: string, text: string): string {
  if (text === "") return "";
  return `<span class="${cls}">${escapeHtml(text)}</span>`;
}

function indentWidth(s: string): number {
  let n = 0;
  for (const c of s) {
    if (c === " ") n++;
    else if (c === "\t") n += 4;
    else break;
  }
  return n;
}

/** 找出不在引号内的注释起始位置，-1 表示没有 */
function commentIndex(line: string): number {
  let quote = "";
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (quote) {
      if (c === "\\" && quote === '"') {
        i++;
        continue;
      }
      if (c === quote) quote = "";
      continue;
    }
    if (c === "'" || c === '"') {
      quote = c;
      continue;
    }
    if (c === "#" && (i === 0 || line[i - 1] === " " || line[i - 1] === "\t")) {
      return i;
    }
  }
  return -1;
}

/** 高亮一段标量/流式内容 */
function highlightScalar(code: string): string {
  let out = "";
  let i = 0;
  while (i < code.length) {
    const c = code[i];
    // 引号字符串
    if (c === "'" || c === '"') {
      const q = c;
      let j = i + 1;
      while (j < code.length) {
        if (code[j] === "\\" && q === '"') {
          j += 2;
          continue;
        }
        if (code[j] === q) {
          if (q === "'" && code[j + 1] === "'") {
            j += 2;
            continue;
          }
          j++;
          break;
        }
        j++;
      }
      out += span("hl-str", code.slice(i, j));
      i = j;
      continue;
    }
    // 动作标签 !task 之类
    if (c === "!") {
      const m = /^!\s*[A-Za-z_][\w-]*/.exec(code.slice(i));
      if (m) {
        out += span("hl-tag", m[0]);
        i += m[0].length;
        continue;
      }
    }
    // 流式映射里的键
    const km = /^([A-Za-z_][\w.\-]*)(\s*:)/.exec(code.slice(i));
    if (km) {
      out += span("hl-key", km[1]) + span("hl-punct", km[2]);
      i += km[0].length;
      continue;
    }
    // 数字
    const nm = /^-?\d+(\.\d+)?/.exec(code.slice(i));
    if (nm) {
      out += span("hl-num", nm[0]);
      i += nm[0].length;
      continue;
    }
    // 布尔 / 空
    const bm = /^(true|false|null|yes|no|on|off|~)\b/.exec(code.slice(i));
    if (bm) {
      out += span("hl-bool", bm[0]);
      i += bm[0].length;
      continue;
    }
    // 括号标点
    if ("{}[],".includes(c)) {
      out += span("hl-punct", c);
      i++;
      continue;
    }
    if (c === "-" && (i === 0 || code[i - 1] === " ")) {
      out += span("hl-dash", c);
      i++;
      continue;
    }
    // 普通文本：连续取到下一个特殊字符
    let j = i;
    while (j < code.length && !"{}[],'\"!".includes(code[j])) {
      j++;
    }
    if (j === i) j = i + 1;
    out += escapeHtml(code.slice(i, j));
    i = j;
  }
  return out;
}

/** YAML 高亮 */
export function highlightYAML(src: string): string {
  const lines = src.split("\n");
  const out: string[] = [];
  let blockIndent = -1;

  for (const line of lines) {
    if (blockIndent >= 0) {
      if (line.trim() === "") {
        out.push("");
        continue;
      }
      if (indentWidth(line) > blockIndent) {
        out.push(span("hl-block", line));
        continue;
      }
      blockIndent = -1;
    }

    // 整行注释 / 空行
    const trimmed = line.trimStart();
    if (trimmed === "") {
      out.push(escapeHtml(line));
      continue;
    }
    if (trimmed.startsWith("#")) {
      out.push(escapeHtml(line.slice(0, line.length - trimmed.length)) + span("hl-comment", trimmed));
      continue;
    }

    const ci = commentIndex(line);
    const codePart = ci >= 0 ? line.slice(0, ci) : line;
    const commentPart = ci >= 0 ? line.slice(ci) : "";
    const lead = codePart.length - codePart.trimStart().length;
    const prefix = codePart.slice(0, lead);
    let rest = codePart.slice(lead);

    let html = escapeHtml(prefix);

    // 列表短横线（可能嵌套）
    while (/^-\s/.test(rest)) {
      html += span("hl-dash", "-") + escapeHtml(rest.slice(1, 2));
      rest = rest.slice(2);
      const extra = rest.length - rest.trimStart().length;
      html += escapeHtml(rest.slice(0, extra));
      rest = rest.slice(extra);
    }

    // 键名
    const km = /^([A-Za-z_][\w.\-]*)(\s*:\s*)([\s\S]*)$/.exec(rest);
    if (km) {
      html += span("hl-key", km[1]) + span("hl-punct", km[2]);
      const value = km[3];
      const vm = /^([|>][+\-\d]*)\s*$/.exec(value.trim());
      if (vm) {
        html += span("hl-punct", value.slice(0, value.length - value.trimStart().length));
        html += span("hl-tag", vm[1]);
        blockIndent = indentWidth(line);
      } else {
        html += highlightScalar(value);
      }
    } else {
      html += highlightScalar(rest);
    }

    if (commentPart) html += span("hl-comment", commentPart);
    out.push(html);
  }
  return out.join("\n");
}

/** XML 高亮 */
export function highlightXML(src: string): string {
  const lines = src.split("\n");
  return lines
    .map((line) => {
      let out = "";
      let i = 0;
      while (i < line.length) {
        const lt = line.indexOf("<", i);
        if (lt < 0) {
          out += escapeHtml(line.slice(i));
          break;
        }
        out += escapeHtml(line.slice(i, lt));
        // 注释
        if (line.startsWith("<!--", lt)) {
          const end = line.indexOf("-->", lt);
          const stop = end < 0 ? line.length : end + 3;
          out += span("hl-comment", line.slice(lt, stop));
          i = stop;
          continue;
        }
        // 处理指令
        if (line.startsWith("<?", lt)) {
          const end = line.indexOf("?>", lt);
          const stop = end < 0 ? line.length : end + 2;
          out += span("hl-punct", line.slice(lt, stop));
          i = stop;
          continue;
        }
        const gt = line.indexOf(">", lt);
        if (gt < 0) {
          out += span("hl-punct", line.slice(lt));
          break;
        }
        const tag = line.slice(lt, gt + 1);
        const nameM = /^<\/?([\w:.\-]+)/.exec(tag);
        const closing = tag.startsWith("</");
        const selfClose = tag.endsWith("/>");
        let inner = "";
        if (nameM) {
          const nameStart = closing ? 2 : 1;
          const name = tag.slice(nameStart, nameStart + nameM[1].length);
          inner += span("hl-punct", tag.slice(0, nameStart)) + span("hl-xmltag", name);
          let restEnd = tag.length - 1 - (selfClose ? 1 : 0);
          const rest = tag.slice(nameStart + name.length, restEnd);
          // 属性
          inner += rest.replace(
            /([\w:.\-]+)(\s*=\s*)("[^"]*"|'[^']*')/g,
            (_m, a, eq, v) => span("hl-attr", a) + span("hl-punct", eq) + span("hl-str", v),
          );
          inner += span("hl-punct", tag.slice(restEnd));
        } else {
          inner = span("hl-punct", tag);
        }
        out += inner;
        i = gt + 1;
      }
      return out;
    })
    .join("\n");
}

/** PowerShell / CMD 高亮（轻量：注释 + 字符串 + 常见关键字） */
export function highlightShell(src: string): string {
  const kw =
    /\b(function|if|else|elseif|foreach|for|while|return|param|try|catch|finally|switch|break|continue|throw|New-Item|Remove-Item|Copy-Item|Get-ChildItem|Get-ItemProperty|Set-ItemProperty|Start-Process|Stop-Process|Test-Path|Join-Path|Add-AppxPackage|Remove-AppxPackage|Get-AppxPackage|Set-Service|Get-Service|reg|setx|robocopy|copy|del|echo|exit)\b/g;
  const lines = src.split("\n");
  return lines
    .map((line) => {
      const t = line.trimStart();
      if (t.startsWith("#") || /^rem\b/i.test(t)) {
        return escapeHtml(line.slice(0, line.length - t.length)) + span("hl-comment", t);
      }
      let s = escapeHtml(line);
      // 字符串
      s = s.replace(/'[^']*'|"[^"]*"/g, (m) => `<span class="hl-str">${m}</span>`);
      // 变量
      s = s.replace(/(\$[\w:]+)/g, '<span class="hl-var">$1</span>');
      // 关键字（避免命中已生成的标签属性）
      s = s.replace(kw, (m) => `<span class="hl-kw">${m}</span>`);
      // 行尾注释
      const ci = commentIndex(line);
      if (ci > 0) {
        const before = s;
        const cut = before.lastIndexOf(escapeHtml(line.slice(ci)));
        if (cut >= 0) {
          s = before.slice(0, cut) + `<span class="hl-comment">${before.slice(cut)}</span>`;
        }
      }
      return s;
    })
    .join("\n");
}

/** JSON 高亮 */
export function highlightJSON(src: string): string {
  let s = escapeHtml(src);
  s = s.replace(/&quot;([^&]*?)&quot;(\s*:)/g, '<span class="hl-key">&quot;$1&quot;</span><span class="hl-punct">$2</span>');
  s = s.replace(/:\s*&quot;(.*?)&quot;/g, ': <span class="hl-str">&quot;$1&quot;</span>');
  s = s.replace(/\b(true|false|null)\b/g, '<span class="hl-bool">$1</span>');
  s = s.replace(/\b(-?\d+(\.\d+)?)\b/g, '<span class="hl-num">$1</span>');
  return s;
}

export type Lang = "yaml" | "xml" | "shell" | "json" | "text";

export function langOf(path: string): Lang {
  const p = path.toLowerCase();
  if (p.endsWith(".yml") || p.endsWith(".yaml")) return "yaml";
  if (p.endsWith(".xml") || p.endsWith(".conf") || p.endsWith(".config")) return "xml";
  if (p.endsWith(".ps1") || p.endsWith(".cmd") || p.endsWith(".bat")) return "shell";
  if (p.endsWith(".json")) return "json";
  return "text";
}

export function highlight(code: string, lang: Lang): string {
  switch (lang) {
    case "yaml":
      return highlightYAML(code);
    case "xml":
      return highlightXML(code);
    case "shell":
      return highlightShell(code);
    case "json":
      return highlightJSON(code);
    default:
      return escapeHtml(code);
  }
}
