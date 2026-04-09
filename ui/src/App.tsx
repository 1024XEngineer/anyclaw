import { Suspense, lazy, useState } from 'react';
import Sidebar from './components/Sidebar';
import Header from './components/Header';
import Dashboard from './components/Dashboard';
import { useHealth } from './hooks/useApi';

const ChannelList = lazy(() => import('./components/ChannelList'));
const SessionList = lazy(() => import('./components/SessionList'));
const CronManager = lazy(() => import('./components/CronManager'));
const ChatPanel = lazy(() => import('./components/ChatPanel'));
const LogsViewer = lazy(() => import('./components/LogsViewer'));

type Page = 'dashboard' | 'channels' | 'sessions' | 'cron' | 'chat' | 'logs';

function App() {
  const [currentPage, setCurrentPage] = useState<Page>('dashboard');
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const { data: health, loading: healthLoading } = useHealth();

  const renderPage = () => {
    switch (currentPage) {
      case 'dashboard':
        return <Dashboard health={health} healthLoading={healthLoading} />;
      case 'channels':
        return <ChannelList />;
      case 'sessions':
        return <SessionList />;
      case 'cron':
        return <CronManager />;
      case 'chat':
        return <ChatPanel />;
      case 'logs':
        return <LogsViewer />;
      default:
        return <Dashboard health={health} healthLoading={healthLoading} />;
    }
  };

  const pageFallback = (
    <div className="space-y-4 animate-pulse">
      <div className="h-8 w-40 rounded bg-gray-200" />
      <div className="h-32 rounded-lg border border-gray-200 bg-white" />
      <div className="h-32 rounded-lg border border-gray-200 bg-white" />
    </div>
  );

  return (
    <div className="flex h-screen bg-gray-100 text-gray-900">
      <Sidebar
        currentPage={currentPage}
        onNavigate={setCurrentPage}
        open={sidebarOpen}
        onToggle={() => setSidebarOpen(!sidebarOpen)}
      />
      <div
        className={`flex-1 flex flex-col overflow-hidden transition-all duration-300 ${
          sidebarOpen ? 'lg:ml-64' : 'lg:ml-16'
        }`}
      >
        <Header
          onToggleSidebar={() => setSidebarOpen(!sidebarOpen)}
          health={health}
          healthLoading={healthLoading}
        />
        <main className="flex-1 overflow-y-auto p-6">
          <Suspense fallback={pageFallback}>
            {renderPage()}
          </Suspense>
        </main>
      </div>
    </div>
  );
}

export default App;
