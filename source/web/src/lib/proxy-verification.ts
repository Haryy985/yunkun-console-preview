const stages: Record<string, string> = {
  tcp: "连接代理端口",
  handshake: "代理握手",
  tls: "TLS 握手",
  response: "等待验证接口响应",
  body: "读取验证响应",
};

const reasons: Record<string, string> = {
  timeout: "超时，请检查当前阶段的连通性",
  canceled: "已取消，可重新验证",
  refused: "连接被拒绝",
  reset: "连接被重置",
  closed: "对端提前关闭连接",
  auth: "代理要求认证或账号密码被拒绝，请检查代理凭据",
  certificate: "证书校验失败，请检查验证地址或代理证书",
  socks_protocol: "SOCKS5 响应不符合协议，请核对端口和协议；端口连通不代表代理可用",
  target_rejected: "SOCKS5 代理无法连接验证地址，可能受访问策略或目标连通性限制",
  connect_rejected: "HTTP 代理拒绝建立 CONNECT 隧道",
  redirect: "验证地址发生重定向，请检查目标地址或认证页面",
  http_status: "验证接口返回非 200 状态",
  oversized: "响应超过 4096 字节，请使用只返回出口 IP 的接口",
  invalid_ip: "响应未包含单个有效 IP，需要纯文本 IP 或 JSON 的 ip/origin 字段",
  connection: "连接失败，请核对协议、认证和验证地址",
};

export function proxyVerificationMessage(value: string): string {
  const match = value.match(/\[proxy_verify:([a-z]+):([a-z_]+)\]/);
  if (match && stages[match[1]] && reasons[match[2]]) {
    const status = value.match(/\bHTTP (\d{3})\b/);
    const prefix = value.includes("目标代理池配置复验失败") ? "目标代理池配置复验失败；" : "";
    return `${prefix}${stages[match[1]]}：${reasons[match[2]]}${status ? `（HTTP ${status[1]}）` : ""}`;
  }
  // Existing records do not contain a stage; don't invent one for old errors.
  if (value.includes("proxy verification timed out")) return "代理验证超时（旧记录未区分阶段），请重新验证查看具体原因";
  if (value.includes("proxy verification SOCKS5")) return "SOCKS5 握手或目标连接失败，请重新验证查看具体原因";
  if (value.includes("proxy verification connection failed")) return "代理验证连接失败，请重新验证查看具体原因";
  return value;
}
