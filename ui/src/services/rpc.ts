import {
  JSONRPCRequest,
  JSONRPCResponse,
  HealthStatus,
  ChannelListResponse,
  Channel,
  SessionListResponse,
  Session,
  CronStatus,
  CronListResponse,
  CronTask,
  CronRunsResponse,
  LogsResponse,
} from '../types';

const isTauriEnvironment = () =>
  typeof window !== 'undefined' &&
  ('__TAURI__' in window || '__TAURI_IPC__' in window || '__TAURI_INTERNALS__' in window);

// Generate a unique ID for each request
const generateId = (): string => {
  return Math.random().toString(36).substring(2, 15);
};

class RPCService {
  private baseURL: string;

  constructor(baseURL?: string) {
    if (baseURL) {
      this.baseURL = baseURL;
      return;
    }

    if (isTauriEnvironment() || import.meta.env.DEV) {
      this.baseURL = 'http://127.0.0.1:28789/rpc';
      return;
    }

    this.baseURL = '/rpc';
  }

  private async call<T = unknown>(method: string, params?: Record<string, unknown>): Promise<T> {
    const request: JSONRPCRequest = {
      jsonrpc: '2.0',
      method,
      params: params || {},
      id: generateId(),
    };

    const response = await fetch(this.baseURL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const rpcResponse: JSONRPCResponse = await response.json();

    if (rpcResponse.error) {
      throw new Error(rpcResponse.error.message);
    }

    return rpcResponse.result as T;
  }

  // Health
  async health(): Promise<HealthStatus> {
    return this.call<HealthStatus>('health');
  }

  // Channels
  async channelsList(): Promise<ChannelListResponse> {
    return this.call<ChannelListResponse>('channels.list');
  }

  async channelsStatus(channelName: string): Promise<Channel> {
    return this.call<Channel>('channels.status', { channel: channelName });
  }

  async sendMessage(channel: string, chatId: string, content: string): Promise<{ success: boolean }> {
    return this.call('send', { channel, chat_id: chatId, content });
  }

  // Sessions
  async sessionsList(): Promise<SessionListResponse> {
    return this.call<SessionListResponse>('sessions.list');
  }

  async sessionsGet(key: string): Promise<Session> {
    return this.call<Session>('sessions.get', { key });
  }

  async sessionsClear(): Promise<{ success: boolean }> {
    return this.call('sessions.clear');
  }

  // Cron
  async cronStatus(): Promise<CronStatus> {
    return this.call<CronStatus>('cron.status');
  }

  async cronList(): Promise<CronListResponse> {
    return this.call<CronListResponse>('cron.list');
  }

  async cronAdd(name: string, cronExpression: string, enabled: boolean): Promise<CronTask> {
    return this.call<CronTask>('cron.add', {
      name,
      cron_expression: cronExpression,
      enabled,
    });
  }

  async cronUpdate(taskId: string, updates: Partial<CronTask>): Promise<CronTask> {
    return this.call<CronTask>('cron.update', {
      id: taskId,
      ...updates,
    });
  }

  async cronRemove(taskId: string): Promise<{ success: boolean }> {
    return this.call('cron.remove', { id: taskId });
  }

  async cronRun(taskId: string): Promise<{ success: boolean }> {
    return this.call('cron.run', { id: taskId });
  }

  async cronRuns(taskId?: string): Promise<CronRunsResponse> {
    return this.call<CronRunsResponse>('cron.runs', taskId ? { id: taskId } : {});
  }

  // Agent
  async agent(channel: string, chatId: string, content: string): Promise<{ success: boolean }> {
    return this.call('agent', { channel, chat_id: chatId, content });
  }

  async agentWait(channel: string, chatId: string, content: string): Promise<{ response: string }> {
    return this.call('agent.wait', { channel, chat_id: chatId, content });
  }

  // Logs
  async logsGet(channel?: string, limit?: number): Promise<LogsResponse> {
    return this.call<LogsResponse>('logs.get', {
      channel,
      limit: limit || 100,
    });
  }
}

export const rpc = new RPCService();
export default RPCService;
