// JSON-RPC Types
export interface JSONRPCRequest {
  jsonrpc: string;
  method: string;
  params?: Record<string, unknown>;
  id?: string;
}

export interface JSONRPCResponse {
  jsonrpc: string;
  result?: unknown;
  error?: JSONRPCError;
  id?: string;
}

export interface JSONRPCError {
  code: number;
  message: string;
  data?: unknown;
}

// Health
export interface HealthStatus {
  status: string;
  time: number;
  timestamp?: number;
  version?: string;
}

// Channel Types
export interface Channel {
  name: string;
  type: string;
  enabled: boolean;
  online?: boolean;
  config?: Record<string, unknown>;
}

export interface ChannelListResponse {
  channels: Channel[];
  count: number;
}

// Session Types
export interface Session {
  key: string;
  channel?: string;
  chat_id?: string;
  user_id?: string;
  created_at: string;
  updated_at?: string;
  messages?: Message[];
  message_count?: number;
}

export interface Message {
  id: string;
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  timestamp: number | string;
  tool_call_id?: string;
  tool_calls?: ToolCall[];
}

export interface ToolCall {
  id: string;
  name: string;
  params?: Record<string, unknown>;
}

export interface SessionListResponse {
  sessions: Session[];
  count: number;
}

// Cron Types
export interface CronTask {
  id: string;
  name: string;
  cron_expression: string;
  enabled: boolean;
  last_run?: number;
  next_run?: number;
  status?: string;
}

export interface CronStatus {
  running: boolean;
  task_count: number;
}

export interface CronListResponse {
  tasks: CronTask[];
  count: number;
}

export interface CronRun {
  id: string;
  task_id: string;
  start_time: number;
  end_time?: number;
  status: 'running' | 'success' | 'failed';
  error?: string;
}

export interface CronRunsResponse {
  runs: CronRun[];
  count: number;
}

// Logs
export interface LogEntry {
  timestamp: number;
  level: string;
  message: string;
  channel?: string;
}

export interface LogsResponse {
  logs: LogEntry[];
  count: number;
}

// WebSocket Message Types
export interface WSMessage {
  jsonrpc: string;
  method?: string;
  params?: Record<string, unknown>;
  result?: unknown;
  error?: JSONRPCError;
  id?: string;
}

// Connection Status
export type ConnectionStatus = 'connected' | 'disconnected' | 'connecting' | 'error';

// API Error
export interface APIError {
  code: number;
  message: string;
  data?: unknown;
}