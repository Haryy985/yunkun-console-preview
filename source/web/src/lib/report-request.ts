export class ReportRequestError extends Error {
  readonly status: number;
  readonly retryAt: number;

  constructor(message: string, status: number, retryAt = 0) {
    super(message);
    this.name = "ReportRequestError";
    this.status = status;
    this.retryAt = retryAt;
  }
}

export function reportRetryAt(header: string | null, now = Date.now()): number {
  const value = header?.trim() ?? "";
  if (/^\d+$/.test(value)) {
    const seconds = Number(value);
    const until = now + seconds * 1000;
    if (Number.isSafeInteger(until)) return until;
  } else if (value.includes(":")) {
    const date = Date.parse(value);
    if (Number.isFinite(date)) return Math.max(now, date);
  }
  // Local duplicate-click backoff when the gateway supplies no valid delay.
  return now + 30_000;
}

export async function requireReportResponse(response: Response, path: string): Promise<Response> {
  if (response.ok) return response;
  const payload: unknown = await response.json().catch(() => null);
  let detail = "";
  let blockers: string[] = [];
  if (payload && typeof payload === "object") {
    const value = payload as { error?: unknown; blockers?: unknown };
    if (typeof value.error === "string") detail = value.error.trim();
    if (Array.isArray(value.blockers)) {
      blockers = value.blockers.filter((item): item is string => typeof item === "string");
    }
  }
  const summary = response.status === 429
    ? "报告请求被限流，请稍后重试"
    : response.status === 401
      ? "登录已失效，请重新登录"
      : response.status === 403
        ? "没有报告访问权限"
        : response.status === 409
          ? "报告导出被阻止"
          : "报告请求失败";
  const reason = detail ? `：${detail}` : "";
  const blocked = blockers.length ? `\n阻断项：${blockers.join("、")}` : "";
  // Never display gateway HTML or include query strings/credentials in diagnostics.
  throw new ReportRequestError(
    `${summary}（HTTP ${response.status}）${reason}${blocked}\n接口：${path.split("?")[0]}`,
    response.status,
    response.status === 429 ? reportRetryAt(response.headers.get("Retry-After")) : 0,
  );
}
