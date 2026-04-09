#![cfg_attr(
    all(not(debug_assertions), target_os = "windows"),
    windows_subsystem = "windows"
)]

use anyclaw_desktop::commands;
use anyclaw_desktop::sidecar::SidecarManager;
use anyclaw_desktop::tray::create_system_tray;
use std::sync::Arc;
use sysinfo::{ProcessStatus, System};
use tokio::sync::Mutex;

/// Check if another AnyClaw instance is already running
fn is_already_running() -> bool {
    let mut sys = System::new_all();
    sys.refresh_all();

    let current_pid = std::process::id();
    let mut count = 0;

    for (pid, process) in sys.processes() {
        let name = process.name();

        // Check for AnyClaw (Tauri app) processes
        let is_anyclaw = name.contains("AnyClaw") || name.contains("anyclaw-desktop");

        // Check if process is running (not zombie/dead)
        let is_running = matches!(
            process.status(),
            ProcessStatus::Run | ProcessStatus::Sleep | ProcessStatus::Idle
        );

        if is_anyclaw && is_running && pid.as_u32() != current_pid {
            count += 1;
            tracing::warn!(
                "Found another AnyClaw instance running: PID={}, name={}",
                pid,
                name
            );
        }
    }

    count > 0
}

fn main() {
    // Initialize logging
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    // Check for single instance - exit if another instance is running
    if is_already_running() {
        tracing::error!("Another AnyClaw instance is already running. Exiting.");
        eprintln!("Another AnyClaw instance is already running. Exiting.");
        std::process::exit(1);
    }

    tracing::info!("Starting AnyClaw Desktop...");

    let sidecar_manager = Arc::new(Mutex::new(SidecarManager::new()));

    tauri::Builder::default()
        .manage(sidecar_manager.clone())
        .system_tray(create_system_tray())
        .on_system_tray_event(|app, event| {
            anyclaw_desktop::tray::handle_tray_event(app, event);
        })
        .setup(move |app| {
            let handle = app.handle();
            let manager = sidecar_manager.clone();

            // Start sidecar on startup
            tauri::async_runtime::spawn(async move {
                let mut mgr = manager.lock().await;
                if let Err(e) = mgr.start(handle.clone()).await {
                    tracing::error!("Failed to start sidecar: {}", e);
                }
            });

            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::get_sidecar_status,
            commands::restart_sidecar,
            commands::open_config_file,
            commands::get_config_path,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
