import { WSMessage, ConnectionStatus } from '../types';

type MessageHandler = (message: WSMessage) => void;
type StatusHandler = (status: ConnectionStatus) => void;

const isTauriEnvironment = () =>
  typeof window !== 'undefined' &&
  ('__TAURI__' in window || '__TAURI_IPC__' in window || '__TAURI_INTERNALS__' in window);

const websocketDebugEnabled = import.meta.env.DEV;

const debugLog = (...args: unknown[]) => {
  if (websocketDebugEnabled) {
    console.log(...args);
  }
};

class WebSocketService {
  private ws: WebSocket | null = null;
  private url: string;
  private messageHandlers: Set<MessageHandler> = new Set();
  private statusHandlers: Set<StatusHandler> = new Set();
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;
  private status: ConnectionStatus = 'disconnected';
  private isConnecting = false;
  private shouldReconnect = true;

  constructor(url: string = '') {
    // 如果没有指定URL，根据环境自动选择
    if (url === '') {
      // 在开发环境中，直接连接到WebSocket服务器（绕过Vite代理）
      // 在生产环境中，使用相对路径
      // 在 Tauri 桌面应用中，始终连接到 localhost
      debugLog('[WebSocket] import.meta.env.DEV:', import.meta.env.DEV);
      debugLog('[WebSocket] isTauri:', isTauriEnvironment());

      if (isTauriEnvironment() || import.meta.env.DEV) {
        this.url = 'ws://127.0.0.1:28789/ws';
        debugLog('[WebSocket] Using direct connection to WebSocket server');
      } else {
        this.url = `ws://${window.location.host}/ws`;
        debugLog('[WebSocket] Using relative path connection');
      }
    } else {
      this.url = url;
    }
    debugLog('WebSocket URL:', this.url);
  }

  connect(): void {
    // 如果已经连接或正在连接，不重复连接
    if (this.ws?.readyState === WebSocket.OPEN || this.isConnecting) {
      return;
    }

    // 关闭旧的 WebSocket 连接（如果存在）
    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      if (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING) {
        this.ws.close();
      }
      this.ws = null;
    }

    this.isConnecting = true;
    this.shouldReconnect = true;
    this.setStatus('connecting');

    try {
      this.ws = new WebSocket(this.url);

      this.ws.onopen = () => {
        debugLog('WebSocket connected, readyState:', this.ws?.readyState);
        this.reconnectAttempts = 0;
        this.isConnecting = false;
        this.setStatus('connected');
      };

      this.ws.onmessage = (event) => {
        try {
          const message: WSMessage = JSON.parse(event.data);
          this.messageHandlers.forEach((handler) => handler(message));
        } catch (error) {
          console.error('Failed to parse WebSocket message:', error);
        }
      };

      this.ws.onclose = (event) => {
        debugLog('WebSocket disconnected:', event.code, event.reason);
        this.isConnecting = false;
        this.setStatus('disconnected');
        if (this.shouldReconnect) {
          this.attemptReconnect();
        }
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        this.isConnecting = false;
      };
    } catch (error) {
      console.error('Failed to create WebSocket:', error);
      this.isConnecting = false;
      this.setStatus('error');
      if (this.shouldReconnect) {
        this.attemptReconnect();
      }
    }
  }

  disconnect(): void {
    this.shouldReconnect = false;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.isConnecting = false;
    this.setStatus('disconnected');
  }

  send(method: string, params?: Record<string, unknown>, id?: string): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      console.error('WebSocket is not connected, current state:', this.ws?.readyState);
      return;
    }

    const message: WSMessage = {
      jsonrpc: '2.0',
      method,
      params: params || {},
      id: id || Math.random().toString(36).substring(2, 15),
    };

    this.ws.send(JSON.stringify(message));
  }

  private attemptReconnect(): void {
    if (!this.shouldReconnect) {
      return;
    }

    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      debugLog('Max reconnect attempts reached');
      this.setStatus('error');
      return;
    }

    this.reconnectAttempts++;
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);

    debugLog(`Attempting to reconnect in ${delay}ms (attempt ${this.reconnectAttempts})`);

    setTimeout(() => {
      if (this.shouldReconnect) {
        this.connect();
      }
    }, delay);
  }

  private setStatus(status: ConnectionStatus): void {
    if (this.status === status) {
      return;
    }
    this.status = status;
    this.statusHandlers.forEach((handler) => handler(status));
  }

  getStatus(): ConnectionStatus {
    return this.status;
  }

  onMessage(handler: MessageHandler): () => void {
    this.messageHandlers.add(handler);
    return () => {
      this.messageHandlers.delete(handler);
    };
  }

  onStatusChange(handler: StatusHandler): () => void {
    this.statusHandlers.add(handler);
    // 立即通知当前状态
    handler(this.status);
    return () => {
      this.statusHandlers.delete(handler);
    };
  }
}

export const wsService = new WebSocketService();
export default WebSocketService;
