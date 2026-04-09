import { useEffect, useState, useCallback } from 'react';
import { wsService } from '../services/websocket';
import { ConnectionStatus, WSMessage } from '../types';

interface UseWebSocketReturn {
  status: ConnectionStatus;
  sendMessage: (method: string, params?: Record<string, unknown>, id?: string) => void;
  lastMessage: WSMessage | null;
}

export const useWebSocket = (autoConnect = true): UseWebSocketReturn => {
  const [status, setStatus] = useState<ConnectionStatus>(wsService.getStatus());
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null);

  useEffect(() => {
    const unsubscribeStatus = wsService.onStatusChange(setStatus);

    if (autoConnect) {
      wsService.connect();
    }

    return () => {
      unsubscribeStatus();
    };
  }, [autoConnect]);

  useEffect(() => {
    const unsubscribeMessage = wsService.onMessage((message) => {
      setLastMessage(message);
    });

    return () => {
      unsubscribeMessage();
    };
  }, []);

  const sendMessage = useCallback((method: string, params?: Record<string, unknown>, id?: string) => {
    wsService.send(method, params, id);
  }, []);

  return {
    status,
    sendMessage,
    lastMessage,
  };
};

export default useWebSocket;