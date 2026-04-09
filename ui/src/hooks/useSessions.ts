import { useState, useEffect, useCallback } from 'react';
import { rpc } from '../services/rpc';
import { Session, SessionListResponse } from '../types';

interface UseSessionsReturn {
  sessions: Session[];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

export const useSessions = (): UseSessionsReturn => {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchSessions = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const response = await rpc.sessionsList();
      // 直接使用数组格式
      const sessionsArray = Array.isArray(response) ? response : (response as SessionListResponse).sessions || [];
      setSessions(sessionsArray);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch sessions');
      setSessions([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

  return {
    sessions,
    loading,
    error,
    refresh: fetchSessions,
  };
};

export default useSessions;
