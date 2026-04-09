import { useState, useEffect } from 'react';
import { useCronList, useCronStatus } from '../hooks/useApi';
import { rpc } from '../services/rpc';
import { CronTask, CronRun } from '../types';

const CronManager: React.FC = () => {
  const { data: cronList, loading: listLoading, execute: refreshList } = useCronList();
  const { data: cronStatus } = useCronStatus();
  const [showAddModal, setShowAddModal] = useState(false);
  const [selectedTask, setSelectedTask] = useState<CronTask | null>(null);
  const [runs, setRuns] = useState<CronRun[]>([]);
  const [loadingRuns, setLoadingRuns] = useState(false);
  const [formData, setFormData] = useState({
    name: '',
    cronExpression: '',
    enabled: true,
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (selectedTask) {
      loadRuns(selectedTask.id);
    }
  }, [selectedTask]);

  const loadRuns = async (taskId: string) => {
    setLoadingRuns(true);
    try {
      const result = await rpc.cronRuns(taskId);
      setRuns(result.runs);
    } catch (err) {
      console.error('Failed to load runs:', err);
    } finally {
      setLoadingRuns(false);
    }
  };

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);

    try {
      await rpc.cronAdd(formData.name, formData.cronExpression, formData.enabled);
      setShowAddModal(false);
      setFormData({ name: '', cronExpression: '', enabled: true });
      refreshList();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add task');
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggle = async (task: CronTask) => {
    try {
      await rpc.cronUpdate(task.id, { enabled: !task.enabled });
      refreshList();
    } catch (err) {
      console.error('Failed to toggle task:', err);
    }
  };

  const handleRun = async (taskId: string) => {
    try {
      await rpc.cronRun(taskId);
      if (selectedTask?.id === taskId) {
        loadRuns(taskId);
      }
    } catch (err) {
      console.error('Failed to run task:', err);
    }
  };

  const handleDelete = async (taskId: string) => {
    if (!confirm('Are you sure you want to delete this task?')) return;

    try {
      await rpc.cronRemove(taskId);
      if (selectedTask?.id === taskId) {
        setSelectedTask(null);
      }
      refreshList();
    } catch (err) {
      console.error('Failed to delete task:', err);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Cron Jobs</h1>
          <p className="text-sm text-gray-500 mt-1">
            Status: {cronStatus?.running ? 'Running' : 'Stopped'} | Tasks: {cronStatus?.task_count || 0}
          </p>
        </div>
        <button
          onClick={() => setShowAddModal(true)}
          className="px-4 py-2 bg-blue-500 hover:bg-blue-600 rounded-lg text-white flex items-center gap-2"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          Add Task
        </button>
      </div>

      {listLoading && !cronList?.tasks.length ? (
        <div className="animate-pulse space-y-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-20 bg-white rounded-lg border border-gray-200"></div>
          ))}
        </div>
      ) : !cronList?.tasks.length ? (
        <div className="bg-white rounded-lg p-8 text-center border border-gray-200">
          <p className="text-gray-500">No cron tasks configured</p>
        </div>
      ) : (
        <div className="grid gap-4">
          {cronList.tasks.map((task) => (
            <div
              key={task.id}
              className={`bg-white rounded-lg p-6 border transition-colors cursor-pointer ${
                selectedTask?.id === task.id ? 'border-blue-500' : 'border-gray-200 hover:border-gray-300'
              }`}
              onClick={() => setSelectedTask(task)}
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4">
                  <div className="p-3 bg-gray-100 rounded-lg">
                    <span className="text-2xl">⏰</span>
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold text-gray-900">{task.name}</h3>
                    <p className="text-sm text-gray-500 font-mono">{task.cron_expression}</p>
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <button
                    onClick={(e) => { e.stopPropagation(); handleToggle(task); }}
                    className={`px-3 py-1 rounded-full text-sm font-medium ${
                      task.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'
                    }`}
                  >
                    {task.enabled ? 'Enabled' : 'Disabled'}
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); handleRun(task.id); }}
                    className="p-2 bg-blue-500 hover:bg-blue-600 rounded-lg text-white"
                    title="Run now"
                  >
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                    </svg>
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); handleDelete(task.id); }}
                    className="p-2 bg-red-500 hover:bg-red-600 rounded-lg text-white"
                    title="Delete"
                  >
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                  </button>
                </div>
              </div>
              <div className="mt-2 text-sm text-gray-400">
                {task.last_run && `Last run: ${new Date(task.last_run * 1000).toLocaleString()}`}
                {task.next_run && ` | Next run: ${new Date(task.next_run * 1000).toLocaleString()}`}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Run History */}
      {selectedTask && (
        <div className="bg-white rounded-lg p-6 border border-gray-200 mt-6">
          <h3 className="text-lg font-semibold text-gray-900 mb-4">
            Run History: {selectedTask.name}
          </h3>
          {loadingRuns ? (
            <div className="animate-pulse space-y-2">
              {[1, 2, 3].map((i) => (
                <div key={i} className="h-12 bg-gray-100 rounded"></div>
              ))}
            </div>
          ) : runs.length === 0 ? (
            <p className="text-gray-500">No runs recorded</p>
          ) : (
            <div className="space-y-2">
              {runs.map((run) => (
                <div
                  key={run.id}
                  className="flex items-center justify-between p-3 bg-gray-50 rounded"
                >
                  <div>
                    <span className="text-sm text-gray-700">
                      {new Date(run.start_time * 1000).toLocaleString()}
                    </span>
                    {run.end_time && (
                      <span className="text-xs text-gray-400 ml-2">
                        ({(run.end_time - run.start_time).toFixed(2)}s)
                      </span>
                    )}
                  </div>
                  <span className={`px-2 py-1 rounded text-xs font-medium ${
                    run.status === 'success' ? 'bg-green-100 text-green-700' :
                    run.status === 'failed' ? 'bg-red-100 text-red-700' :
                    'bg-yellow-100 text-yellow-700'
                  }`}>
                    {run.status}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Add Modal */}
      {showAddModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-full max-w-md border border-gray-200">
            <h2 className="text-xl font-bold text-gray-900 mb-4">Add Cron Task</h2>
            {error && (
              <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded text-red-600 text-sm">
                {error}
              </div>
            )}
            <form onSubmit={handleAdd} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-600 mb-1">Name</label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  className="w-full px-3 py-2 bg-gray-50 border border-gray-300 rounded-lg text-gray-900"
                  required
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-600 mb-1">Cron Expression</label>
                <input
                  type="text"
                  value={formData.cronExpression}
                  onChange={(e) => setFormData({ ...formData, cronExpression: e.target.value })}
                  placeholder="* * * * *"
                  className="w-full px-3 py-2 bg-gray-50 border border-gray-300 rounded-lg text-gray-900 font-mono"
                  required
                />
                <p className="text-xs text-gray-400 mt-1">Format: minute hour day month weekday</p>
              </div>
              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="enabled"
                  checked={formData.enabled}
                  onChange={(e) => setFormData({ ...formData, enabled: e.target.checked })}
                  className="w-4 h-4 rounded bg-gray-50 border-gray-300"
                />
                <label htmlFor="enabled" className="text-sm text-gray-600">Enable immediately</label>
              </div>
              <div className="flex justify-end gap-2 pt-4">
                <button
                  type="button"
                  onClick={() => { setShowAddModal(false); setError(null); }}
                  className="px-4 py-2 bg-gray-200 hover:bg-gray-300 rounded-lg text-gray-700"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting}
                  className="px-4 py-2 bg-blue-500 hover:bg-blue-600 disabled:bg-gray-400 rounded-lg text-white"
                >
                  {submitting ? 'Adding...' : 'Add'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};

export default CronManager;