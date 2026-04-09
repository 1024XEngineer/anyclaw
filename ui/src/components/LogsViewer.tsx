import { useDeferredValue, useEffect, useRef, useState } from 'react';
import { useLogs } from '../hooks/useApi';

const LogsViewer: React.FC = () => {
  const { data, loading, execute } = useLogs();
  const [filter, setFilter] = useState('');
  const [levelFilter, setLevelFilter] = useState<string>('all');
  const [channelFilter, setChannelFilter] = useState<string>('all');
  const [autoRefresh, setAutoRefresh] = useState(false);
  const refreshIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const deferredFilter = useDeferredValue(filter);

  useEffect(() => {
    if (autoRefresh) {
      refreshIntervalRef.current = setInterval(() => {
        void execute();
      }, 5000);
    } else if (refreshIntervalRef.current) {
      clearInterval(refreshIntervalRef.current);
      refreshIntervalRef.current = null;
    }

    return () => {
      if (refreshIntervalRef.current) {
        clearInterval(refreshIntervalRef.current);
        refreshIntervalRef.current = null;
      }
    };
  }, [autoRefresh, execute]);

  const normalizedFilter = deferredFilter.trim().toLowerCase();

  const getLevelColor = (level: string) => {
    const l = level.toLowerCase();
    if (l === 'error' || l === 'err') return 'text-red-600';
    if (l === 'warn' || l === 'warning') return 'text-yellow-600';
    if (l === 'info') return 'text-blue-600';
    if (l === 'debug') return 'text-gray-500';
    return 'text-gray-700';
  };

  const filteredLogs = data?.logs.filter((log) => {
    if (levelFilter !== 'all' && log.level.toLowerCase() !== levelFilter) {
      return false;
    }
    if (channelFilter !== 'all' && log.channel !== channelFilter) {
      return false;
    }
    if (normalizedFilter && !log.message.toLowerCase().includes(normalizedFilter)) {
      return false;
    }
    return true;
  }) || [];

  const uniqueChannels = Array.from(
    new Set(
      (data?.logs ?? [])
        .map((log) => log.channel)
        .filter((channel): channel is string => Boolean(channel)),
    ),
  ).sort();

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Logs</h1>
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-2 text-sm text-gray-500">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
              className="w-4 h-4 rounded bg-gray-50 border-gray-300"
            />
            Auto-refresh
          </label>
          <button
            onClick={() => void execute()}
            className="px-4 py-2 bg-gray-200 hover:bg-gray-300 rounded-lg text-gray-700 flex items-center gap-2"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            Refresh
          </button>
        </div>
      </div>

      {/* Filters */}
      <div className="bg-white rounded-lg p-4 border border-gray-200">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-600 mb-1">Search</label>
            <input
              type="text"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Search logs..."
              className="w-full px-3 py-2 bg-gray-50 border border-gray-300 rounded-lg text-gray-900"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-600 mb-1">Level</label>
            <select
              value={levelFilter}
              onChange={(e) => setLevelFilter(e.target.value)}
              className="w-full px-3 py-2 bg-gray-50 border border-gray-300 rounded-lg text-gray-900"
            >
              <option value="all">All Levels</option>
              <option value="error">Error</option>
              <option value="warn">Warning</option>
              <option value="info">Info</option>
              <option value="debug">Debug</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-600 mb-1">Channel</label>
            <select
              value={channelFilter}
              onChange={(e) => setChannelFilter(e.target.value)}
              className="w-full px-3 py-2 bg-gray-50 border border-gray-300 rounded-lg text-gray-900"
            >
              <option value="all">All Channels</option>
              {uniqueChannels.map((channel) => (
                <option key={channel} value={channel}>
                  {channel}
                </option>
              ))}
            </select>
          </div>
          <div className="flex items-end">
            <div className="text-sm text-gray-500">
              Showing {filteredLogs.length} of {data?.count || 0} logs
            </div>
          </div>
        </div>
      </div>

      {/* Logs */}
      <div className="bg-white rounded-lg border border-gray-200">
        {loading && !data?.logs.length ? (
          <div className="p-8">
            <div className="animate-pulse space-y-2">
              {[1, 2, 3, 4, 5].map((i) => (
                <div key={i} className="h-6 bg-gray-200 rounded"></div>
              ))}
            </div>
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className="p-8 text-center text-gray-500">
            No logs found
          </div>
        ) : (
          <div className="max-h-[600px] overflow-y-auto">
            <table className="w-full">
              <thead className="bg-gray-50 sticky top-0">
                <tr>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Time</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Level</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Channel</th>
                  <th className="px-4 py-2 text-left text-sm font-medium text-gray-500">Message</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {filteredLogs.map((log, idx) => (
                  <tr key={idx} className="hover:bg-gray-50">
                    <td className="px-4 py-2 text-sm text-gray-400 font-mono whitespace-nowrap">
                      {new Date(log.timestamp * 1000).toLocaleTimeString()}
                    </td>
                    <td className="px-4 py-2 text-sm font-medium font-mono">
                      <span className={getLevelColor(log.level)}>{log.level}</span>
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-500">
                      {log.channel || '-'}
                    </td>
                    <td className="px-4 py-2 text-sm text-gray-700 font-mono">
                      {log.message}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};

export default LogsViewer;
