import { useState, useCallback, useEffect } from 'react';
import { rpc } from '../services/rpc';

interface UseApiReturn<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
  execute: (...args: unknown[]) => Promise<T | null>;
  reset: () => void;
}

export const useApi = <T,>(apiCall: (...args: unknown[]) => Promise<T>): UseApiReturn<T> => {
  const [data, setData] = useState<T | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const execute = useCallback(async (...args: unknown[]): Promise<T | null> => {
    setLoading(true);
    setError(null);

    try {
      const result = await apiCall(...args);
      setData(result);
      return result;
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'An error occurred';
      setError(errorMessage);
      return null;
    } finally {
      setLoading(false);
    }
  }, [apiCall]);

  const reset = useCallback(() => {
    setData(null);
    setLoading(false);
    setError(null);
  }, []);

  return {
    data,
    loading,
    error,
    execute,
    reset,
  };
};

const fetchHealth = () => rpc.health();
const fetchChannels = () => rpc.channelsList();
const fetchSessions = () => rpc.sessionsList();
const fetchCronStatus = () => rpc.cronStatus();
const fetchCronList = () => rpc.cronList();
const fetchLogs = () => rpc.logsGet();

const useAutoApi = <T,>(apiCall: (...args: unknown[]) => Promise<T>): UseApiReturn<T> => {
  const api = useApi(apiCall);

  useEffect(() => {
    void api.execute();
  }, [api.execute]);

  return api;
};

// Pre-configured hooks for common API calls
// These hooks automatically fetch data on mount

export const useHealth = () => useAutoApi(fetchHealth);

export const useChannels = () => useAutoApi(fetchChannels);

export const useSessions = () => useAutoApi(fetchSessions);

export const useCronStatus = () => useAutoApi(fetchCronStatus);

export const useCronList = () => useAutoApi(fetchCronList);

export const useLogs = () => useAutoApi(fetchLogs);

export default useApi;
