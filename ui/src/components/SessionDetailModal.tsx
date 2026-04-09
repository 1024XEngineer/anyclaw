import { useState, useEffect } from 'react';
import { rpc } from '../services/rpc';
import { Session, Message } from '../types';

interface SessionDetailModalProps {
  session: Session;
  onClose: () => void;
}

const SessionDetailModal: React.FC<SessionDetailModalProps> = ({ session, onClose }) => {
  const [messages, setMessages] = useState<Message[]>(session.messages || []);
  const [loading, setLoading] = useState(!session.messages);
  const [error, setError] = useState<string | null>(null);

  // 解析 session key
  const keyParts = session.key.split('__');
  const channelName = keyParts[0] || 'unknown';
  const sessionId = keyParts[1] || 'unknown';

  useEffect(() => {
    const fetchSessionDetail = async () => {
      if (session.messages && session.messages.length > 0) {
        return;
      }

      setLoading(true);
      setError(null);

      try {
        const response = await rpc.sessionsGet(session.key);
        setMessages(response.messages || []);
      } catch (err) {
        console.error('Failed to fetch session detail:', err);
        setError(err instanceof Error ? err.message : 'Failed to fetch session details');
      } finally {
        setLoading(false);
      }
    };

    fetchSessionDetail();
  }, [session.key, session.messages]);

  // 处理点击背景关闭
  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
      onClick={handleBackdropClick}
    >
      <div className="bg-white rounded-xl w-full max-w-4xl max-h-[80vh] flex flex-col border border-gray-200 shadow-2xl">
        {/* 头部 */}
        <div className="flex items-center justify-between p-6 border-b border-gray-200">
          <div>
            <h2 className="text-xl font-bold text-gray-900">
              Session Details
            </h2>
            <p className="text-sm text-gray-500 mt-1">
              ID: {sessionId} | Channel: {channelName}
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-100 rounded-lg transition-colors"
          >
            <svg className="w-5 h-5 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* 基本信息 */}
        <div className="p-6 border-b border-gray-100 bg-gray-50">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="text-xs text-gray-500 uppercase tracking-wide">Created</p>
              <p className="text-sm text-gray-900 mt-1">
                {session.created_at ? new Date(session.created_at).toLocaleString() : 'N/A'}
              </p>
            </div>
            {session.updated_at && (
              <div>
                <p className="text-xs text-gray-500 uppercase tracking-wide">Updated</p>
                <p className="text-sm text-gray-900 mt-1">
                  {new Date(session.updated_at).toLocaleString()}
                </p>
              </div>
            )}
            {session.user_id && (
              <div>
                <p className="text-xs text-gray-500 uppercase tracking-wide">User ID</p>
                <p className="text-sm text-gray-900 mt-1 truncate">{session.user_id}</p>
              </div>
            )}
            {session.chat_id && (
              <div>
                <p className="text-xs text-gray-500 uppercase tracking-wide">Chat ID</p>
                <p className="text-sm text-gray-900 mt-1 truncate">{session.chat_id}</p>
              </div>
            )}
          </div>
        </div>

        {/* 消息列表 */}
        <div className="flex-1 overflow-y-auto p-6">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">
            Messages ({messages.length})
          </h3>

          {loading && (
            <div className="flex items-center justify-center py-8">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
            </div>
          )}

          {error && (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-600">
              {error}
            </div>
          )}

          {!loading && !error && messages.length === 0 && (
            <div className="bg-gray-50 rounded-lg p-8 text-center">
              <p className="text-gray-500">No messages in this session</p>
            </div>
          )}

          {!loading && !error && messages.length > 0 && (
            <div className="space-y-4">
              {messages.map((message, index) => (
                <div
                  key={message.id || index}
                  className={`rounded-lg p-4 border ${
                    message.role === 'user'
                      ? 'bg-blue-50 border-blue-200'
                      : message.role === 'assistant'
                      ? 'bg-green-50 border-green-200'
                      : message.role === 'tool'
                      ? 'bg-purple-50 border-purple-200'
                      : 'bg-gray-50 border-gray-200'
                  }`}
                >
                  <div className="flex items-center justify-between mb-2">
                    <span
                      className={`text-xs font-semibold px-2 py-1 rounded ${
                        message.role === 'user'
                          ? 'bg-blue-500 text-white'
                          : message.role === 'assistant'
                          ? 'bg-green-500 text-white'
                          : message.role === 'tool'
                          ? 'bg-purple-500 text-white'
                          : 'bg-gray-500 text-white'
                      }`}
                    >
                      {message.role}
                    </span>
                    <span className="text-xs text-gray-400">
                      {message.timestamp
                        ? (() => {
                            const ts = Number(message.timestamp);
                            const date = !isNaN(ts)
                              ? new Date(ts)
                              : new Date(String(message.timestamp));
                            if (!isNaN(date.getTime())) {
                              const pad = (n: number) => n.toString().padStart(2, '0');
                              return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
                            }
                            return String(message.timestamp);
                          })()
                        : ''}
                    </span>
                  </div>
                  <p className="text-gray-800 whitespace-pre-wrap break-words">
                    {message.content}
                  </p>
                  {message.tool_calls && message.tool_calls.length > 0 && (
                    <div className="mt-3 space-y-2">
                      <p className="text-xs text-gray-500 font-semibold">Tool Calls:</p>
                      {message.tool_calls.map((toolCall, tcIndex) => (
                        <div
                          key={toolCall.id || tcIndex}
                          className="bg-yellow-50 border border-yellow-200 rounded p-3 text-sm"
                        >
                          <div className="flex items-center gap-2 mb-1">
                            <span className="text-yellow-700 font-mono text-xs">
                              {toolCall.name}
                            </span>
                            <span className="text-gray-400 text-xs">ID: {toolCall.id}</span>
                          </div>
                          {toolCall.params && (
                            <pre className="text-gray-600 text-xs overflow-x-auto whitespace-pre-wrap">
                              {JSON.stringify(toolCall.params, null, 2)}
                            </pre>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* 底部操作栏 */}
        <div className="p-4 border-t border-gray-200 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 bg-gray-600 hover:bg-gray-700 text-white rounded-lg transition-colors"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};

export default SessionDetailModal;
