import { useState } from 'react';
import { useSessions } from '../hooks/useSessions';
import { rpc } from '../services/rpc';
import { Session } from '../types';
import SessionDetailModal from './SessionDetailModal';

const SessionList: React.FC = () => {
  const { sessions, loading, error, refresh } = useSessions();
  const [clearing, setClearing] = useState(false);
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);

  const handleClear = async () => {
    if (!confirm('Are you sure you want to clear all sessions?')) {
      return;
    }

    setClearing(true);
    try {
      await rpc.sessionsClear();
      refresh();
    } catch (err) {
      console.error('Failed to clear sessions:', err);
    } finally {
      setClearing(false);
    }
  };

  if (loading && !(sessions?.length)) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold text-gray-900">Sessions</h1>
        </div>
        <div className="animate-pulse space-y-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-24 bg-white rounded-lg border border-gray-200"></div>
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Sessions</h1>
        <div className="flex gap-2">
          <button
            onClick={refresh}
            className="px-4 py-2 bg-gray-200 hover:bg-gray-300 text-gray-700 rounded-lg flex items-center gap-2"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            Refresh
          </button>
          <button
            onClick={handleClear}
            disabled={clearing || !(sessions?.length)}
            className="px-4 py-2 bg-red-500 hover:bg-red-600 disabled:bg-gray-400 text-white rounded-lg flex items-center gap-2"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
            {clearing ? 'Clearing...' : 'Clear All'}
          </button>
        </div>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-600">
          {error}
        </div>
      )}

      {!sessions?.length ? (
        <div className="bg-white rounded-lg p-8 text-center border border-gray-200">
          <p className="text-gray-500">No sessions found</p>
        </div>
      ) : (
        <div className="space-y-4">
          {sessions?.map((session, index) => {
            // 解析 key: channel__sessionid
            const keyParts = (session?.key || '').split('__');
            const channelName = keyParts[0] || 'unknown';
            const sessionId = keyParts[1] || 'unknown';

            return (
              <div
                key={session?.key || index}
                className="bg-white rounded-lg p-6 border border-gray-200 hover:border-blue-500 hover:shadow-md cursor-pointer transition-all"
                onClick={() => setSelectedSession(session)}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className="p-3 bg-gray-100 rounded-lg">
                      <span className="text-2xl">💬</span>
                    </div>
                    <div>
                      <h3 className="text-lg font-semibold text-gray-900">
                        Session {sessionId}
                      </h3>
                      <p className="text-sm text-gray-500">
                        Channel: {channelName}
                      </p>
                    </div>
                  </div>
                  <div className="text-right">
                    <p className="text-sm text-gray-500">
                      Created: {session?.created_at ? new Date(session.created_at).toLocaleString() : 'N/A'}
                    </p>
                    <p className="text-sm text-gray-400">
                      {session?.message_count || 0} messages
                    </p>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {selectedSession && (
        <SessionDetailModal
          session={selectedSession}
          onClose={() => setSelectedSession(null)}
        />
      )}
    </div>
  );
};

export default SessionList;