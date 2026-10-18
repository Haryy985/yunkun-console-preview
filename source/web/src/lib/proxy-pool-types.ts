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
  quota: ProxyQuota;
};

export type ProxyGlobalSettings = {
  enabled: boolean;
  default_pool_id?: number;
};

export type ProxyNode = {
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
  binding?: { task_id: number; pool_id: number; node_id?: number; mode: string };
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
  protocol: "http" | "https" | "socks5";
  host: string;
  port: number;
  metadata: Record<string, unknown>;
  state: "pending" | "verifying" | "verified" | "approved" | "ignored" | "invalid";
  authorized: boolean;
  authorization_status?: "allowed" | "unconfigured" | "out_of_scope";
  verified_ip?: string;
  verified_at?: string;
  latency_ms?: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
};
