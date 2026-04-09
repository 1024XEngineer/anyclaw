use crate::sidecar::SidecarStatus;
use std::path::PathBuf;
use tauri::State;

/// Get the current sidecar status
#[tauri::command]
pub async fn get_sidecar_status(
    manager: State<'_, std::sync::Arc<tokio::sync::Mutex<crate::sidecar::SidecarManager>>>,
    app: tauri::AppHandle,
) -> Result<SidecarStatus, String> {
    let mut mgr = manager.lock().await;
    Ok(mgr.refresh_status(&app).await)
}

/// Restart the sidecar
#[tauri::command]
pub async fn restart_sidecar<R: tauri::Runtime>(
    app: tauri::AppHandle<R>,
    manager: State<'_, std::sync::Arc<tokio::sync::Mutex<crate::sidecar::SidecarManager>>>,
) -> Result<(), String> {
    let mut mgr = manager.lock().await;
    mgr.restart(app)
        .await
        .map_err(|e| format!("Failed to restart sidecar: {}", e))
}

/// Open the config file in the default editor
#[tauri::command]
pub async fn open_config_file() -> Result<(), String> {
    let config_path = get_config_path_internal()?;

    // Use system's default handler to open the file
    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .arg(&config_path)
            .spawn()
            .map_err(|e| format!("Failed to open config file: {}", e))?;
    }

    #[cfg(target_os = "windows")]
    {
        std::process::Command::new("cmd")
            .args(["/C", "start", &config_path.to_string_lossy()])
            .spawn()
            .map_err(|e| format!("Failed to open config file: {}", e))?;
    }

    #[cfg(target_os = "linux")]
    {
        std::process::Command::new("xdg-open")
            .arg(&config_path)
            .spawn()
            .map_err(|e| format!("Failed to open config file: {}", e))?;
    }

    Ok(())
}

/// Get the config file path
#[tauri::command]
pub fn get_config_path() -> Result<String, String> {
    get_config_path_internal()
        .map(|p| p.to_string_lossy().to_string())
}

fn get_config_path_internal() -> Result<PathBuf, String> {
    let home = dirs::home_dir().ok_or_else(|| "Cannot determine home directory".to_string())?;

    // Check for existing config
    let config_paths = vec![
        home.join(".anyclaw").join("config.json"),
        home.join(".anyclaw").join("config.yaml"),
        home.join(".anyclaw").join("config.yml"),
    ];

    for path in config_paths {
        if path.exists() {
            return Ok(path);
        }
    }

    // Return default path (even if doesn't exist)
    Ok(home.join(".anyclaw").join("config.json"))
}
