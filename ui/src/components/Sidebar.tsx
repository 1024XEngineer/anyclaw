type Page = 'dashboard' | 'channels' | 'sessions' | 'cron' | 'chat' | 'logs';

interface SidebarProps {
  currentPage: Page;
  onNavigate: (page: Page) => void;
  open: boolean;
  onToggle: () => void;
}

const menuItems: { id: Page; label: string; icon: string }[] = [
  { id: 'dashboard', label: 'Dashboard', icon: '📊' },
  { id: 'channels', label: 'Channels', icon: '📡' },
  { id: 'sessions', label: 'Sessions', icon: '💬' },
  { id: 'cron', label: 'Cron Jobs', icon: '⏰' },
  { id: 'chat', label: 'Chat', icon: '🗨️' },
  { id: 'logs', label: 'Logs', icon: '📜' },
];

const Sidebar: React.FC<SidebarProps> = ({ currentPage, onNavigate, open, onToggle }) => {
  return (
    <>
      {/* Mobile overlay */}
      {open && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 z-20 lg:hidden"
          onClick={onToggle}
        />
      )}

      {/* Sidebar */}
      <aside
        className={`
          fixed top-0 left-0 z-30 h-full bg-white border-r border-gray-200
          transition-all duration-300 ease-in-out
          ${open ? 'w-64' : 'w-16'}
          ${open ? 'translate-x-0' : '-translate-x-full lg:translate-x-0 lg:w-16'}
        `}
      >
        {/* Header */}
        <div className="flex items-center justify-between h-16 px-4 border-b border-gray-200">
          {open && (
            <h1 className="text-xl font-bold text-gray-900">AnyClaw</h1>
          )}
          <button
            onClick={onToggle}
            className="p-2 rounded-md hover:bg-gray-100 lg:hidden"
          >
            <svg
              className="w-5 h-5 text-gray-600"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        {/* Navigation */}
        <nav className="p-2 space-y-1">
          {menuItems.map((item) => (
            <button
              key={item.id}
              onClick={() => onNavigate(item.id)}
              className={`
                w-full flex items-center gap-3 px-3 py-2.5 rounded-lg
                transition-colors duration-200
                ${
                  currentPage === item.id
                    ? 'bg-blue-500 text-white'
                    : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'
                }
              `}
            >
              <span className="text-xl">{item.icon}</span>
              {open && <span className="font-medium">{item.label}</span>}
            </button>
          ))}
        </nav>
      </aside>

      {/* Toggle button for collapsed state */}
      {!open && (
        <button
          onClick={onToggle}
          className="fixed top-4 left-4 z-40 p-2 bg-white rounded-lg border border-gray-200 lg:hidden"
        >
          <svg
            className="w-5 h-5 text-gray-600"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M4 6h16M4 12h16M4 18h16"
            />
          </svg>
        </button>
      )}
    </>
  );
};

export default Sidebar;