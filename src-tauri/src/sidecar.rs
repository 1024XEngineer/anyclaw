use anyhow::{Context, Result};
use std::collections::{HashMap, VecDeque};
use std::env;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;
use tauri::api::process::{Command, CommandChild, CommandEvent};
use tauri::Manager;
use tauri::{AppHandle, Runtime};
use tokio::net::TcpStream;
use tokio::sync::Mutex;
use tokio::time::sleep;

const GATEWAY_ADDR: &str = "127.0.0.1:28789";
const DASHBOARD_URL: &str = "http://127.0.0.1:28789/dashboard/";
const SIDECAR_ARGS: &[&str] = &["start"];

/// Sidecar status
#[derive(Debug, Clone, serde::Serialize, PartialEq)]
pub enum SidecarStatus {
    Starting,
    Running,
    Stopped,
    Error(String),
}

/// Manages the anyclaw sidecar process
pub struct SidecarManager {
    process: Option<CommandChild>,
    status: SidecarStatus,
    recent_output: VecDeque<String>,
}

impl SidecarManager {
    pub fn new() -> Self {
        Self {
            process: None,
            status: SidecarStatus::Stopped,
            recent_output: VecDeque::with_capacity(32),
        }
    }

    /// Start the anyclaw gateway sidecar
    pub async fn start<R: Runtime>(&mut self, app: AppHandle<R>) -> Result<()> {
        // Check if already running (by checking port)
        if self.is_port_in_use().await {
            tracing::info!("Gateway already running on port 28789, skipping startup");
            self.status = SidecarStatus::Running;
            self.emit_status(&app);
            self.navigate_to_dashboard(&app);
            return Ok(());
        }

        if self.process.is_some() {
            tracing::warn!("Sidecar process tracked but not running, clearing...");
            self.process = None;
        }

        self.status = SidecarStatus::Starting;
        self.recent_output.clear();
        self.emit_status(&app);

        tracing::info!("Starting anyclaw sidecar...");

        let sidecar_env = self.sidecar_env();
        let sidecar_cwd = self.sidecar_working_dir();
        let sidecar_program = self.sidecar_program_path()?;

        // Use Tauri's Command API for sidecar
        let (mut rx, child) = Command::new(sidecar_program.to_string_lossy().to_string())
            .args(SIDECAR_ARGS)
            .envs(sidecar_env)
            .current_dir(sidecar_cwd)
            .spawn()
            .context("Failed to spawn sidecar")?;

        self.process = Some(child);
        tracing::info!("Sidecar process spawned, waiting for ready...");

        // Spawn a task to log sidecar output
        let manager = app.state::<Arc<Mutex<SidecarManager>>>().inner().clone();
        let app_for_events = app.clone();
        tauri::async_runtime::spawn(async move {
            while let Some(event) = rx.recv().await {
                match event {
                    CommandEvent::Stdout(line) => {
                        tracing::info!("[sidecar stdout] {}", line);
                        let mut mgr = manager.lock().await;
                        mgr.push_output(format!("stdout: {}", line));
                    }
                    CommandEvent::Stderr(line) => {
                        tracing::warn!("[sidecar stderr] {}", line);
                        let mut mgr = manager.lock().await;
                        mgr.push_output(format!("stderr: {}", line));
                    }
                    CommandEvent::Terminated(payload) => {
                        tracing::warn!("[sidecar] terminated with code: {:?}", payload.code);
                        let mut mgr = manager.lock().await;
                        mgr.push_output(format!("terminated: code={:?}", payload.code));
                        mgr.process = None;
                        if matches!(mgr.status, SidecarStatus::Starting) {
                            let recent_output = mgr.recent_output_summary();
                            let message = if recent_output.is_empty() {
                                format!("Sidecar terminated before becoming ready (code: {:?})", payload.code)
                            } else {
                                format!(
                                    "Sidecar terminated before becoming ready (code: {:?})\n\nRecent sidecar output:\n{}",
                                    payload.code, recent_output
                                )
                            };
                            mgr.status = SidecarStatus::Error(message);
                        } else if matches!(mgr.status, SidecarStatus::Running) {
                            mgr.status = SidecarStatus::Stopped;
                        }
                        mgr.emit_status(&app_for_events);
                        break;
                    }
                    CommandEvent::Error(error) => {
                        tracing::error!("[sidecar error] {}", error);
                    }
                    _ => {}
                }
            }
        });

        // Wait for the gateway to be ready
        match self.wait_for_ready().await {
            Ok(_) => {
                self.status = SidecarStatus::Running;
                self.emit_status(&app);
                self.navigate_to_dashboard(&app);
                tracing::info!("Sidecar is ready");
                Ok(())
            }
            Err(e) => {
                let recent_output = self.recent_output_summary();
                let error_message = if recent_output.is_empty() {
                    format!("Health check failed: {}", e)
                } else {
                    format!("Health check failed: {}\n\nRecent sidecar output:\n{}", e, recent_output)
                };
                self.status = SidecarStatus::Error(error_message);
                self.emit_status(&app);
                Err(e)
            }
        }
    }

    /// Check if port 28789 is already in use
    async fn is_port_in_use(&self) -> bool {
        TcpStream::connect(GATEWAY_ADDR).await.is_ok()
    }

    /// Stop the sidecar process
    pub async fn stop(&mut self) -> Result<()> {
        if let Some(child) = self.process.take() {
            tracing::info!("Stopping sidecar...");
            child.kill().context("Failed to kill sidecar process")?;
            self.status = SidecarStatus::Stopped;
            tracing::info!("Sidecar stopped");
        }
        Ok(())
    }

    /// Restart the sidecar
    pub async fn restart<R: Runtime>(&mut self, app: AppHandle<R>) -> Result<()> {
        self.stop().await?;
        sleep(Duration::from_secs(1)).await;
        self.start(app).await
    }

    /// Wait for the gateway to be ready
    async fn wait_for_ready(&self) -> Result<()> {
        let max_attempts = 60;
        let delay_ms = 500;

        for attempt in 1..=max_attempts {
            tracing::info!("Health check attempt {}/{}", attempt, max_attempts);

            match TcpStream::connect(GATEWAY_ADDR).await {
                Ok(_) => {
                    tracing::info!("Gateway is ready");
                    return Ok(());
                }
                Err(e) => {
                    if attempt == max_attempts {
                        return Err(anyhow::anyhow!(
                            "Gateway not ready after {} attempts: {}",
                            max_attempts,
                            e
                        ));
                    }
                    sleep(Duration::from_millis(delay_ms)).await;
                }
            }
        }

        Err(anyhow::anyhow!("Gateway not ready"))
    }

    /// Get current status
    pub async fn refresh_status<R: Runtime>(&mut self, app: &AppHandle<R>) -> SidecarStatus {
        if self.is_port_in_use().await {
            if self.status != SidecarStatus::Running {
                self.status = SidecarStatus::Running;
                self.emit_status(app);
                self.navigate_to_dashboard(app);
            }
        }

        self.status.clone()
    }

    pub fn status(&self) -> SidecarStatus {
        self.status.clone()
    }

    /// Emit status to frontend
    fn emit_status<R: Runtime>(&self, app: &AppHandle<R>) {
        let _ = app.emit_all("sidecar-status", self.status.clone());
    }

    fn push_output(&mut self, line: String) {
        if self.recent_output.len() >= 24 {
            self.recent_output.pop_front();
        }
        self.recent_output.push_back(line);
    }

    fn recent_output_summary(&self) -> String {
        self.recent_output
            .iter()
            .cloned()
            .collect::<Vec<_>>()
            .join("\n")
    }

    fn sidecar_env(&self) -> HashMap<String, String> {
        let mut envs = HashMap::new();

        if let Ok(home) = env::var("HOME") {
            envs.insert("HOME".to_string(), home);
        } else if let Some(home) = dirs::home_dir() {
            envs.insert("HOME".to_string(), home.to_string_lossy().into_owned());
        }

        if let Ok(path) = env::var("PATH") {
            envs.insert("PATH".to_string(), path);
        }

        envs
    }

    fn sidecar_working_dir(&self) -> PathBuf {
        dirs::home_dir().unwrap_or_else(|| env::current_dir().unwrap_or_else(|_| PathBuf::from(".")))
    }

    fn sidecar_program_path(&self) -> Result<PathBuf> {
        #[cfg(debug_assertions)]
        {
            let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
            let path = manifest_dir
                .join("binaries")
                .join(Self::sidecar_binary_name());
            return Ok(path);
        }

        #[cfg(not(debug_assertions))]
        {
            let exe_dir = env::current_exe()
                .context("Failed to determine current executable path")?
                .parent()
                .map(PathBuf::from)
                .context("Failed to determine executable directory")?;
            Ok(exe_dir.join(Self::sidecar_binary_name()))
        }
    }

    fn sidecar_binary_name() -> String {
        let suffix = if cfg!(target_os = "windows") { ".exe" } else { "" };
        format!("anyclaw-{}{}", Self::target_triple(), suffix)
    }

    fn target_triple() -> &'static str {
        #[cfg(all(target_os = "macos", target_arch = "aarch64"))]
        {
            "aarch64-apple-darwin"
        }
        #[cfg(all(target_os = "macos", target_arch = "x86_64"))]
        {
            "x86_64-apple-darwin"
        }
        #[cfg(all(target_os = "linux", target_arch = "x86_64"))]
        {
            "x86_64-unknown-linux-gnu"
        }
        #[cfg(all(target_os = "windows", target_arch = "x86_64"))]
        {
            "x86_64-pc-windows-msvc"
        }
    }

    fn navigate_to_dashboard<R: Runtime>(&self, app: &AppHandle<R>) {
        if let Some(window) = app.get_window("main") {
            let script = format!(
                "if (window.location.href !== '{url}') {{ window.location.replace('{url}'); }}",
                url = DASHBOARD_URL
            );

            if let Err(error) = window.eval(&script) {
                tracing::warn!("Failed to navigate to dashboard: {}", error);
            }
        }
    }
}

impl Default for SidecarManager {
    fn default() -> Self {
        Self::new()
    }
}
