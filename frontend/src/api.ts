import * as PlaybookService from "../bindings/playbookstudio/playbookservice";

export { PlaybookService };

/** 统一的错误信息提取 */
export function errText(e: unknown): string {
  if (e == null) return "未知错误";
  if (typeof e === "string") return e;
  if (typeof e === "object") {
    const anyE = e as Record<string, unknown>;
    if (typeof anyE.message === "string") return anyE.message;
  }
  return String(e);
}

/** 等待后端返回，null 视为错误 */
export async function call<T>(p: Promise<T | null | undefined>): Promise<T> {
  const r = await p;
  if (r === null || r === undefined) {
    throw new Error("后端没有返回结果");
  }
  return r as T;
}
