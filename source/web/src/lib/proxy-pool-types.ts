export type ProxyPoolConfig = {
  name: string;
  kind: "fixed" | "xiaoxiang" | "generic";
  enabled: boolean;
  protocol: "http" | "https" | "socks5";
  addresses: string[];
  api_url?: string;
  items_path?: string;
  host_field?: string;
  port_field?: string;
  ttl_seconds: number;
  fetch_count: number;
  safety_seconds: number;
  quota_supported: boolean;
  quota_key_style?: "app" | "user";
  low_quota: number;
  verify_url?: string;
  expiry_warning_seconds: number;
};

export type ProxySecrets = {
  app_key?: string;
  app_secret?: string;
  proxy_username?: string;
  proxy_password?: string;
  api_header?: string;
  api_value?: string;
};

export type ProxyQuota = {
  unavailable?: boolean;
  supported: boolean;
  total?: number;
  remaining?: number;
  checked_at?: string;
  error?: string;
};

export type ProxyPool = {
  id: number;
  config: ProxyPoolConfig;
  has_credentials: boolean;
  credentials_available?: boolean;
  quota: ProxyQuota;
};

export type ProxyGlobalSettings = {
  enabled: boolean;
  default_pool_id?: number;
};

export type ProxyNode = {
  runtime_cooldown_until?: string;
  runtime_error?: string;
  id: number;
  pool_id: number;
  address: string;
  declared_ip?: string;
  expires_at?: string;
  verified_ip?: string;
  verified_at?: string;
  last_error?: string;
  enabled: boolean;
  origin?: "provider" | "manual" | "fofa" | "quake" | string;
  last_checked_at?: string;
  latency_ms?: number;
  success_count: number;
  failure_count: number;
  last_success_at?: string;
  last_failure_at?: string;
  notes?: string;
  deleted_at?: string;
  last_verify_mode?: ProxyVerificationMode;
  last_verify_target?: string;
  last_http_status?: number;
};

export type ProxyVerificationMode = "connectivity" | "exit_ip";

export type ProxyVerificationResult = {
  ip?: string;
  mode: ProxyVerificationMode;
  target: string;
  http_status?: number;
  latency_ms: number;
  checked_at: string;
};

export type ProxyNodeQuery = {
  page: number;
  page_size: number;
  q?: string;
  status?: string;
  protocol?: string;
  sort?: string;
};

export type ProxyNodeSummary = {
  total: number;
  available: number;
  pending: number;
  expiring: number;
  error: number;
  disabled: number;
};

export type ProxyEvent = {
  id: number;
  task_id?: number;
  pool_id?: number;
  node_id?: number;
  target: string;
  result: string;
  traffic_id?: string;
  revision?: number;
  created_at: string;
};

export type TaskNetwork = {
  bound: boolean;
  mode?: string;
  enforced: boolean;
  pool_name?: string;
  status?: string;
  reason?: string;
  binding?: { task_id: number; pool_id: number; node_id?: number; mode: string; strategy?: "round_robin" | "fixed" };
  routing_strategy?: "round_robin" | "fixed";
  node?: ProxyNode;
  events: ProxyEvent[];
  unmanaged_tools?: string[];
  capture?: string;
};

export type ProxyDiscoverySource = {
  next_offset: number;
  exhausted: boolean;
  id: number;
  provider: "fofa" | "quake";
  name: string;
  query: string;
  authorized_scope: string;
  verify_url: string;
  fetch_limit: number;
  enabled: boolean;
  has_credentials: boolean;
  last_run_at?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type ProxyDiscoveryResult = {
  id: number;
  source_id: number;
  address: string;
  protocol: "http" | "https" | "socks5" | "unknown";
  host: string;
  port: number;
  metadata: Record<string, unknown>;
  state: "pending" | "verifying" | "verified" | "approved" | "ignored" | "invalid";
  authorized: boolean;
  authorization_status?: "allowed" | "unconfigured" | "out_of_scope";
  last_verify_mode?: ProxyVerificationMode;
  last_verify_target?: string;
  last_http_status?: number;
  last_checked_at?: string;
  verified_ip?: string;
  verified_at?: string;
  latency_ms?: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type ProxyDiscoveryRunSummary = {
  run: {
    id: number;
    source_id: number;
    offset_start: number;
    requested: number;
    returned: number;
    parsed: number;
    inserted: number;
    authorized: number;
    unknown_protocol: number;
    created_at: string;
  };
  attempted: number;
  succeeded: number;
  states: Record<string, number>;
  failures: { stage: string; code: string; count: number }[];
};
