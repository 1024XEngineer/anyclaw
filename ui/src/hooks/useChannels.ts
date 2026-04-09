import { useState, useEffect, useCallback } from 'react';
import { rpc } from '../services/rpc';
import { Channel, ChannelListResponse } from '../types';

interface UseChannelsReturn {
  channels: Channel[];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

export const useChannels = (): UseChannelsReturn => {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchChannels = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const response: ChannelListResponse = await rpc.channelsList();
      setChannels(response.channels);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch channels');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchChannels();

    // Refresh channels every 30 seconds
    const interval = setInterval(fetchChannels, 30000);

    return () => clearInterval(interval);
  }, [fetchChannels]);

  return {
    channels,
    loading,
    error,
    refresh: fetchChannels,
  };
};

export default useChannels;