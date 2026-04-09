import { useChannels } from '../hooks/useChannels';
import StatusIndicator from './StatusIndicator';

const ChannelList: React.FC = () => {
  const { channels, loading, error, refresh } = useChannels();

  if (loading && !channels.length) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold text-gray-900">Channels</h1>
        </div>
        <div className="animate-pulse space-y-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-20 bg-white rounded-lg border border-gray-200"></div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold text-gray-900">Channels</h1>
          <button
            onClick={refresh}
            className="px-4 py-2 bg-blue-500 hover:bg-blue-600 rounded-lg text-white"
          >
            Retry
          </button>
        </div>
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-600">
          {error}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Channels</h1>
        <button
          onClick={refresh}
          className="px-4 py-2 bg-gray-200 hover:bg-gray-300 text-gray-700 rounded-lg flex items-center gap-2"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Refresh
        </button>
      </div>

      {channels.length === 0 ? (
        <div className="bg-white rounded-lg p-8 text-center border border-gray-200">
          <p className="text-gray-500">No channels configured</p>
        </div>
      ) : (
        <div className="grid gap-4">
          {channels.map((channel) => {
            // 兼容后端返回字符串数组的情况
            const channelName = typeof channel === 'string' ? channel : (channel.name ?? '');
            const channelType = typeof channel === 'object' ? channel.type : channelName.split(':')[0];
            const channelEnabled = typeof channel === 'object' ? channel.enabled : true;
            const channelOnline = typeof channel === 'object' ? !!channel.online : false;
            const channelConfig = typeof channel === 'object' ? channel.config : undefined;

            return (
              <div
                key={channelName}
                className="bg-white rounded-lg p-6 border border-gray-200 hover:border-blue-400 transition-colors"
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className="p-3 bg-gray-100 rounded-lg">
                      <span className="text-2xl">📡</span>
                    </div>
                    <div>
                      <h3 className="text-lg font-semibold text-gray-900 capitalize">
                        {channelName}
                      </h3>
                      <p className="text-sm text-gray-500">Type: {channelType}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-4">
                    <div className="text-right">
                      <p className="text-sm text-gray-500">Status</p>
                      <StatusIndicator status={channelOnline} />
                    </div>
                    <div className={`px-3 py-1 rounded-full text-sm font-medium ${
                      channelEnabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'
                    }`}>
                      {channelEnabled ? 'Enabled' : 'Disabled'}
                    </div>
                  </div>
                </div>

                {channelConfig && Object.keys(channelConfig).length > 0 && (
                  <div className="mt-4 pt-4 border-t border-gray-100">
                    <h4 className="text-sm font-medium text-gray-500 mb-2">Configuration</h4>
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                      {Object.entries(channelConfig).slice(0, 4).map(([key, value]) => (
                        <div key={key} className="bg-gray-50 rounded p-2">
                          <p className="text-xs text-gray-400 capitalize">{key}</p>
                          <p className="text-sm text-gray-700 truncate">{String(value)}</p>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

export default ChannelList;