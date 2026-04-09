import { useState, useEffect, useRef } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
import { Message } from '../types';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter';
import bash from 'react-syntax-highlighter/dist/esm/languages/prism/bash';
import go from 'react-syntax-highlighter/dist/esm/languages/prism/go';
import javascript from 'react-syntax-highlighter/dist/esm/languages/prism/javascript';
import json from 'react-syntax-highlighter/dist/esm/languages/prism/json';
import jsx from 'react-syntax-highlighter/dist/esm/languages/prism/jsx';
import markdown from 'react-syntax-highlighter/dist/esm/languages/prism/markdown';
import python from 'react-syntax-highlighter/dist/esm/languages/prism/python';
import sql from 'react-syntax-highlighter/dist/esm/languages/prism/sql';
import tsx from 'react-syntax-highlighter/dist/esm/languages/prism/tsx';
import typescript from 'react-syntax-highlighter/dist/esm/languages/prism/typescript';
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';

SyntaxHighlighter.registerLanguage('bash', bash);
SyntaxHighlighter.registerLanguage('shell', bash);
SyntaxHighlighter.registerLanguage('sh', bash);
SyntaxHighlighter.registerLanguage('go', go);
SyntaxHighlighter.registerLanguage('javascript', javascript);
SyntaxHighlighter.registerLanguage('js', javascript);
SyntaxHighlighter.registerLanguage('json', json);
SyntaxHighlighter.registerLanguage('jsx', jsx);
SyntaxHighlighter.registerLanguage('markdown', markdown);
SyntaxHighlighter.registerLanguage('md', markdown);
SyntaxHighlighter.registerLanguage('python', python);
SyntaxHighlighter.registerLanguage('py', python);
SyntaxHighlighter.registerLanguage('sql', sql);
SyntaxHighlighter.registerLanguage('tsx', tsx);
SyntaxHighlighter.registerLanguage('typescript', typescript);
SyntaxHighlighter.registerLanguage('ts', typescript);
SyntaxHighlighter.registerLanguage('yaml', yaml);
SyntaxHighlighter.registerLanguage('yml', yaml);

// 复制按钮组件
const CopyButton: React.FC<{ code: string }> = ({ code }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <button
      onClick={handleCopy}
      className="absolute top-2 right-2 p-1.5 rounded bg-gray-600 hover:bg-gray-500 transition-colors"
      title={copied ? '已复制' : '复制代码'}
    >
      {copied ? (
        <svg className="w-4 h-4 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
        </svg>
      ) : (
        <svg className="w-4 h-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
        </svg>
      )}
    </button>
  );
};

// 消息复制按钮组件
const MessageCopyButton: React.FC<{ content: string }> = ({ content }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <button
      onClick={handleCopy}
      className="mt-1 p-1 rounded hover:bg-gray-200 transition-colors self-end"
      title={copied ? '已复制' : '复制内容'}
    >
      {copied ? (
        <svg className="w-4 h-4 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
        </svg>
      ) : (
        <svg className="w-4 h-4 text-gray-400 hover:text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
        </svg>
      )}
    </button>
  );
};

interface ChatEvent {
  run_id: string;
  seq: number;
  state: 'delta' | 'thinking' | 'tool' | 'final' | 'error';
  content: string;
  timestamp: number;
}

interface StreamingMessage {
  id: string;
  runId: string;
  content: string;
  thinking: string;
  toolCalls: { name: string; args?: string; result?: string }[];
  isComplete: boolean;
}

const ChatPanel: React.FC = () => {
  const { status: wsStatus, lastMessage, sendMessage } = useWebSocket(true);
  const [messages, setMessages] = useState<Message[]>([]);
  const [message, setMessage] = useState('');
  const [sending, setSending] = useState(false);
  const [sessionId, setSessionId] = useState<string>('');
  const [streamingMessage, setStreamingMessage] = useState<StreamingMessage | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const sendingTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 监听连接建立，获取session_id
  useEffect(() => {
    if (lastMessage?.method === 'connected') {
      const params = lastMessage.params as { session_id: string };
      setSessionId(params.session_id);
    }
  }, [lastMessage]);

  // 处理消息响应
  useEffect(() => {
    // 监听聊天事件
    if (lastMessage?.method === 'chat.event') {
      const params = lastMessage.params as { data?: ChatEvent };
      const event = params.data;
      if (!event) return;

      if (event.state === 'delta') {
        // 增量文本
        setStreamingMessage((prev) => {
          if (!prev || prev.runId !== event.run_id) {
            return {
              id: event.run_id,
              runId: event.run_id,
              content: event.content,
              thinking: '',
              toolCalls: [],
              isComplete: false,
            };
          }
          return {
            ...prev,
            content: prev.content + event.content,
          };
        });
      } else if (event.state === 'thinking') {
        // 思考过程
        setStreamingMessage((prev) => {
          if (!prev || prev.runId !== event.run_id) {
            return {
              id: event.run_id,
              runId: event.run_id,
              content: '',
              thinking: event.content,
              toolCalls: [],
              isComplete: false,
            };
          }
          return {
            ...prev,
            thinking: prev.thinking + event.content,
          };
        });
      } else if (event.state === 'tool') {
        // 工具调用
        try {
          const toolInfo = JSON.parse(event.content);
          setStreamingMessage((prev) => {
            if (!prev || prev.runId !== event.run_id) {
              return {
                id: event.run_id,
                runId: event.run_id,
                content: '',
                thinking: '',
                toolCalls: [toolInfo],
                isComplete: false,
              };
            }
            return {
              ...prev,
              toolCalls: [...prev.toolCalls, toolInfo],
            };
          });
        } catch {
          // 忽略解析错误
        }
      } else if (event.state === 'final') {
        // 最终消息
        const finalContent = event.content;

        // 清除超时定时器
        if (sendingTimeoutRef.current) {
          clearTimeout(sendingTimeoutRef.current);
          sendingTimeoutRef.current = null;
        }

        // 添加到消息列表
        const assistantMessage: Message = {
          id: event.run_id,
          role: 'assistant',
          content: finalContent,
          timestamp: event.timestamp / 1000,
        };
        setMessages((prev) => [...prev, assistantMessage]);

        // 清除流式消息
        setStreamingMessage(null);
        setSending(false);
      } else if (event.state === 'error') {
        // 错误
        console.error('Chat error:', event.content);

        // 清除超时定时器
        if (sendingTimeoutRef.current) {
          clearTimeout(sendingTimeoutRef.current);
          sendingTimeoutRef.current = null;
        }

        setStreamingMessage(null);
        setSending(false);
      }
    }

    // 监听来自gateway的消息（旧格式兼容）
    if (lastMessage?.method === 'chat.response') {
      // 清除超时定时器
      if (sendingTimeoutRef.current) {
        clearTimeout(sendingTimeoutRef.current);
        sendingTimeoutRef.current = null;
      }

      const params = lastMessage.params as { data?: { content: string }; content?: string };
      const content = params.data?.content || params.content;
      if (content) {
        const assistantMessage: Message = {
          id: Date.now().toString(),
          role: 'assistant',
          content: content,
          timestamp: Date.now() / 1000,
        };
        setMessages((prev) => [...prev, assistantMessage]);
      }
      // 重置发送状态
      setSending(false);
    }
  }, [lastMessage]);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, streamingMessage]);

  // 组件卸载时清理定时器
  useEffect(() => {
    return () => {
      if (sendingTimeoutRef.current) {
        clearTimeout(sendingTimeoutRef.current);
      }
    };
  }, []);

  // 发送消息
  const handleSend = async () => {
    if (!message.trim()) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      role: 'user',
      content: message,
      timestamp: Date.now() / 1000,
    };

    setMessages((prev) => [...prev, userMessage]);
    setSending(true);
    setMessage('');

    // 清除之前的超时定时器
    if (sendingTimeoutRef.current) {
      clearTimeout(sendingTimeoutRef.current);
    }

    // 设置超时保护，60秒后自动重置发送状态
    sendingTimeoutRef.current = setTimeout(() => {
      setSending(false);
      setStreamingMessage(null);
    }, 60000);

    // 发送消息（不等待响应，通过事件接收）
    sendMessage('chat', { content: userMessage.content });
  };

  const clearChat = () => {
    setMessages([]);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Chat</h1>
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${wsStatus === 'connected' ? 'bg-green-500' : 'bg-red-500'}`} />
          <span className="text-sm text-gray-500">{wsStatus}</span>
          {sessionId && (
            <span className="text-xs text-gray-400 ml-2">
              Session: {sessionId.substring(0, 8)}...
            </span>
          )}
        </div>
      </div>

      {/* Messages */}
      <div className="bg-white rounded-lg border border-gray-200 h-[500px] flex flex-col">
        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {messages.length === 0 ? (
            <div className="h-full flex items-center justify-center text-gray-400">
              <div className="text-center">
                <p>直接与 Gateway 对话</p>
                <p className="text-sm mt-2">消息通过 WebSocket 实时传输</p>
              </div>
            </div>
          ) : (
            messages.map((msg) => (
              <div
                key={msg.id}
                className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                <div
                  className={`max-w-[70%] p-3 rounded-lg ${
                    msg.role === 'user'
                      ? 'bg-blue-500 text-white'
                      : 'bg-gray-100 text-gray-800'
                  }`}
                >
                  <div className={`text-xs mb-1 capitalize ${msg.role === 'user' ? 'text-blue-200' : 'text-gray-500'}`}>
                    {msg.role}
                  </div>
                  {msg.role === 'assistant' ? (
                  <div className="flex flex-col">
                    <div className="prose prose-sm max-w-none dark:prose-invert">
                      <ReactMarkdown
                        remarkPlugins={[remarkGfm]}
                        components={{
                          code({ className, children }) {
                            const match = /language-(\w+)/.exec(className || '');
                            const isInline = !match;
                            const codeString = String(children).replace(/\n$/, '');

                            return isInline ? (
                              <code className="px-1.5 py-0.5 bg-gray-700 text-green-400 rounded text-sm font-mono">
                                {codeString}
                              </code>
                            ) : (
                              <div className="relative">
                                <SyntaxHighlighter
                                  style={oneDark}
                                  language={match[1]}
                                  PreTag="div"
                                  customStyle={{
                                    margin: 0,
                                    borderRadius: '0.5rem',
                                    fontSize: '0.875rem',
                                    paddingTop: '2.5rem',
                                  }}
                                >
                                  {codeString}
                                </SyntaxHighlighter>
                                <CopyButton code={codeString} />
                              </div>
                            );
                          },
                        }}
                      >
                        {msg.content}
                      </ReactMarkdown>
                    </div>
                    <MessageCopyButton content={msg.content} />
                  </div>
                ) : (
                  <p className="whitespace-pre-wrap">{msg.content}</p>
                )}
                </div>
              </div>
            ))
          )}
          <div ref={messagesEndRef} />
        </div>

        {/* Input */}
        <div className="p-4 border-t border-gray-200">
          <div className="flex gap-2">
            <input
              type="text"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyPress={(e) => e.key === 'Enter' && !e.shiftKey && handleSend()}
              placeholder="输入消息..."
              disabled={sending || wsStatus !== 'connected'}
              className="flex-1 px-3 py-2 bg-gray-50 border border-gray-300 rounded-lg text-gray-900 disabled:opacity-50"
            />
            <button
              onClick={handleSend}
              disabled={sending || !message.trim() || wsStatus !== 'connected'}
              className="px-4 py-2 bg-blue-500 hover:bg-blue-600 disabled:bg-gray-400 rounded-lg text-white"
            >
              {sending ? '发送中...' : '发送'}
            </button>
            <button
              onClick={clearChat}
              className="px-4 py-2 bg-gray-200 hover:bg-gray-300 rounded-lg text-gray-700"
            >
              清空
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ChatPanel;
