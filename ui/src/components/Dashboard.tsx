import { useCronStatus } from '../hooks/useApi';
import { useChannels } from '../hooks/useChannels';
import StatusIndicator from './StatusIndicator';
import { Channel, HealthStatus } from '../types';

interface DashboardProps {
  health: HealthStatus | null;
  healthLoading: boolean;
}

const Dashboard: React.FC<DashboardProps> = ({ health, healthLoading }) => {
  const { channels, loading: channelsLoading } = useChannels();
  const { data: cronStatus } = useCronStatus();

  // 兼容 channel 数据，可能是字符串或对象
  const getChannelName = (channel: Channel | string) => {
    return typeof channel === 'string' ? channel : (channel.name ?? '');
  };

  const getChannelType = (channel: Channel | string) => {
    if (typeof channel === 'string') return channel.split(':')[0];
    return channel.type ?? '';
  };

  const getChannelOnline = (channel: Channel | string) => {
    if (typeof channel === 'string') return false;
    return channel.online ?? false;
  };

  const stats = [
    {
      label: 'Channels',
      value: channels?.length || 0,
      subValue: `${(channels?.filter((c) => getChannelOnline(c)) || []).length || 0} online`,
      icon: '📡',
      color: 'blue',
    },
    {
      label: 'Cron Tasks',
      value: cronStatus?.task_count || 0,
      subValue: cronStatus?.running ? 'Running' : 'Stopped',
      icon: '⏰',
      color: 'green',
    },
    {
      label: 'WebSocket',
      value: 'Active',
      subValue: 'Real-time',
      icon: '🔌',
      color: 'yellow',
    },
  ];

  const getColorClasses = (color: string) => {
    const colors: Record<string, string> = {
      blue: 'bg-blue-500',
      green: 'bg-green-500',
      purple: 'bg-purple-500',
      yellow: 'bg-yellow-500',
    };
    return colors[color] || 'bg-gray-500';
  };

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-gray-900">Dashboard</h1>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {stats.map((stat) => (
          <div
            key={stat.label}
            className="bg-white rounded-lg p-6 border border-gray-200 shadow-sm"
          >
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-gray-500">{stat.label}</p>
                {statLoading(stat.label) ? (
                  <div className="animate-pulse">
                    <div className="h-8 bg-gray-200 rounded w-12"></div>
                  </div>
                ) : (
                  <p className="text-2xl font-bold text-gray-900 mt-1">{stat.value}</p>
                )}
                <p className="text-xs text-gray-400 mt-1">{stat.subValue}</p>
              </div>
              <div className={`p-3 rounded-lg ${getColorClasses(stat.color)}`}>
                <span className="text-2xl">{stat.icon}</span>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Status Section */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Health Status */}
        <div className="bg-white rounded-lg p-6 border border-gray-200 shadow-sm">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">System Health</h3>
          {healthLoading ? (
            <div className="animate-pulse space-y-2">
              <div className="h-4 bg-gray-200 rounded w-3/4"></div>
              <div className="h-4 bg-gray-200 rounded w-1/2"></div>
            </div>
          ) : health ? (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <span className="text-gray-500">Status</span>
                <StatusIndicator status={health.status === 'ok'} />
              </div>
              <div className="flex items-center justify-between">
                <span className="text-gray-500">Last Check</span>
                <span className="text-gray-900">
                  {new Date((health.time || health.timestamp || 0) * 1000).toLocaleString()}
                </span>
              </div>
            </div>
          ) : (
            <p className="text-red-500">Unable to fetch health status</p>
          )}
        </div>

        {/* Quick Stats */}
        <div className="bg-white rounded-lg p-6 border border-gray-200 shadow-sm">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">Quick Stats</h3>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-gray-500">Total Channels</span>
              <span className="text-gray-900 font-medium">{channels?.length || 0}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-gray-500">Online Channels</span>
              <span className="text-green-600 font-medium">
                {(channels?.filter((c) => getChannelOnline(c)) || []).length || 0}
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-gray-500">Cron Tasks</span>
              <span className="text-gray-900 font-medium">{cronStatus?.task_count || 0}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Channel Status Overview */}
      <div className="bg-white rounded-lg p-6 border border-gray-200 shadow-sm">
        <h3 className="text-lg font-semibold text-gray-900 mb-4">Channel Status</h3>
        {channelsLoading ? (
          <div className="animate-pulse grid grid-cols-2 md:grid-cols-4 gap-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="h-20 bg-gray-200 rounded-lg"></div>
            ))}
          </div>
        ) : channels && channels.length > 0 ? (
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            {channels.map((channel: Channel | string, idx: number) => (
              <div
                key={getChannelName(channel) || idx}
                className="bg-gray-50 rounded-lg p-4 flex items-center justify-between"
              >
                <div>
                  <p className="font-medium text-gray-900 capitalize">{getChannelName(channel)}</p>
                  <p className="text-sm text-gray-500">{getChannelType(channel)}</p>
                </div>
                <StatusIndicator status={getChannelOnline(channel)} />
              </div>
            ))}
          </div>
        ) : (
          <p className="text-gray-500">No channels configured</p>
        )}
      </div>
    </div>
  );

  function statLoading(label: string): boolean {
    if (label === 'Channels') return channelsLoading;
    return false;
  }
};

export default Dashboard;
