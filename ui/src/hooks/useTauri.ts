import { useEffect, useState, useCallback } from 'react';

const isTauriEnvironment = () =>
  typeof window !== 'undefined' &&
  ('__TAURI__' in window || '__TAURI_IPC__' in window || '__TAURI_INTERNALS__' in window);

// Check if running in Tauri environment
export function useIsTauri(): boolean {
  const [isTauri, setIsTauri] = useState(false);

  useEffect(() => {
    setIsTauri(isTauriEnvironment());
  }, []);

  return isTauri;
}

// Hook for Tauri commands
export function useTauriCommands() {
  const isTauri = useIsTauri();

  const getSidecarStatus = useCallback(async (): Promise<string> => {
    if (!isTauri) return 'browser';

    try {
      const { invoke } = await import('@tauri-apps/api/tauri');
      return await invoke('get_sidecar_status');
    } catch (error) {
      console.error('Failed to get sidecar status:', error);
      return 'error';
    }
  }, [isTauri]);

  const restartSidecar = useCallback(async (): Promise<void> => {
    if (!isTauri) return;

    try {
      const { invoke } = await import('@tauri-apps/api/tauri');
      await invoke('restart_sidecar');
    } catch (error) {
      console.error('Failed to restart sidecar:', error);
      throw error;
    }
  }, [isTauri]);

  const openConfigFile = useCallback(async (): Promise<void> => {
    if (!isTauri) return;

    try {
      const { invoke } = await import('@tauri-apps/api/tauri');
      await invoke('open_config_file');
    } catch (error) {
      console.error('Failed to open config file:', error);
      throw error;
    }
  }, [isTauri]);

  const getConfigPath = useCallback(async (): Promise<string | null> => {
    if (!isTauri) return null;

    try {
      const { invoke } = await import('@tauri-apps/api/tauri');
      return await invoke('get_config_path');
    } catch (error) {
      console.error('Failed to get config path:', error);
      return null;
    }
  }, [isTauri]);

  return {
    isTauri,
    getSidecarStatus,
    restartSidecar,
    openConfigFile,
    getConfigPath,
  };
}

// Hook for listening to Tauri events
export function useTauriEvent<T = unknown>(eventName: string, handler: (payload: T) => void) {
  const isTauri = useIsTauri();

  useEffect(() => {
    if (!isTauri) return;

    let unlisten: (() => void) | undefined;

    const setupListener = async () => {
      try {
        const { listen } = await import('@tauri-apps/api/event');
        unlisten = await listen<T>(eventName, (event) => {
          handler(event.payload);
        });
      } catch (error) {
        console.error(`Failed to setup event listener for ${eventName}:`, error);
      }
    };

    setupListener();

    return () => {
      if (unlisten) {
        unlisten();
      }
    };
  }, [isTauri, eventName, handler]);
}

// Hook for window controls
export function useWindowControls() {
  const isTauri = useIsTauri();

  const minimize = useCallback(async () => {
    if (!isTauri) return;

    try {
      const { appWindow } = await import('@tauri-apps/api/window');
      await appWindow.minimize();
    } catch (error) {
      console.error('Failed to minimize window:', error);
    }
  }, [isTauri]);

  const maximize = useCallback(async () => {
    if (!isTauri) return;

    try {
      const { appWindow } = await import('@tauri-apps/api/window');
      await appWindow.toggleMaximize();
    } catch (error) {
      console.error('Failed to maximize window:', error);
    }
  }, [isTauri]);

  const close = useCallback(async () => {
    if (!isTauri) return;

    try {
      const { appWindow } = await import('@tauri-apps/api/window');
      await appWindow.close();
    } catch (error) {
      console.error('Failed to close window:', error);
    }
  }, [isTauri]);

  return {
    isTauri,
    minimize,
    maximize,
    close,
  };
}
