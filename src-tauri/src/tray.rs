use tauri::{
    AppHandle, CustomMenuItem, Manager, Runtime, SystemTray, SystemTrayEvent, SystemTrayMenu,
    SystemTrayMenuItem,
};

/// Create the system tray
pub fn create_system_tray() -> SystemTray {
    let open = CustomMenuItem::new("open".to_string(), "Open AnyClaw");
    let restart = CustomMenuItem::new("restart".to_string(), "Restart Backend");
    let quit = CustomMenuItem::new("quit".to_string(), "Quit");

    let tray_menu = SystemTrayMenu::new()
        .add_item(open)
        .add_native_item(SystemTrayMenuItem::Separator)
        .add_item(restart)
        .add_native_item(SystemTrayMenuItem::Separator)
        .add_item(quit);

    SystemTray::new().with_menu(tray_menu)
}

/// Handle system tray events
pub fn handle_tray_event<R: Runtime>(app: &AppHandle<R>, event: SystemTrayEvent) {
    match event {
        SystemTrayEvent::LeftClick { .. } => {
            // Show the main window on left click
            if let Some(window) = app.get_window("main") {
                let _ = window.show();
                let _ = window.set_focus();
            }
        }
        SystemTrayEvent::MenuItemClick { id, .. } => match id.as_str() {
            "open" => {
                if let Some(window) = app.get_window("main") {
                    let _ = window.show();
                    let _ = window.set_focus();
                }
            }
            "restart" => {
                let app_handle = app.clone();
                tauri::async_runtime::spawn(async move {
                    if let Some(manager) = app_handle.try_state::<std::sync::Arc<tokio::sync::Mutex<crate::sidecar::SidecarManager>>>() {
                        let manager_clone = manager.clone();
                        let mut mgr = manager_clone.lock().await;
                        if let Err(e) = mgr.restart(app_handle.clone()).await {
                            tracing::error!("Failed to restart sidecar: {}", e);
                        }
                    }
                });
            }
            "quit" => {
                // Stop the sidecar before quitting
                let app_handle = app.clone();
                tauri::async_runtime::spawn(async move {
                    if let Some(manager) = app_handle.try_state::<std::sync::Arc<tokio::sync::Mutex<crate::sidecar::SidecarManager>>>() {
                        let manager_clone = manager.clone();
                        let mut mgr = manager_clone.lock().await;
                        let _ = mgr.stop().await;
                    }
                    // Exit after cleanup
                    std::process::exit(0);
                });
            }
            _ => {}
        },
        _ => {}
    }
}
