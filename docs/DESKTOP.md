# AnyClaw Desktop

## 最简单的 V2 方案

AnyClaw 的桌面版现在采用 `Wails` 做原生窗口壳，真正的业务界面仍然是现有的 Gateway Dashboard。

这版的启动流程是：

1. 启动桌面窗口
2. 检查本地 Gateway 是否已经在运行
3. 如果没有运行，就在桌面进程里拉起 Gateway
4. 等待 `http://127.0.0.1:<port>/healthz` 就绪
5. 窗口直接跳转到 `/dashboard`

当前桌面壳还支持一个最小桌宠态：

- 正常态：完整 Dashboard 工作台
- 桌宠态：顶部小窗，只显示桌宠动作和当前状态
- 进入方式：点击“收起为桌宠”
- 恢复方式：双击桌宠

这样做的好处是：

- 不需要重写现有 React 控制台
- 不需要把现有 HTTP / WebSocket API 改成 Wails 专用桥接
- CLI、Gateway、Desktop 共用一套 Go runtime

## 目录

- `cmd/anyclaw-desktop/`: Wails 原生桌面入口
- `cmd/anyclaw-desktop/frontend/`: 桌面壳加载页
- `scripts/build-desktop.ps1`: Windows 便携版构建脚本

## 本地开发

在仓库根目录执行：

```powershell
go build -o anyclaw-desktop.exe .\cmd\anyclaw-desktop
.\anyclaw-desktop.exe
```

如果你已经安装了 Wails CLI，也可以进入桌面目录后执行：

```powershell
cd .\cmd\anyclaw-desktop
wails dev
```

## 打包

Windows 下直接执行：

```powershell
.\scripts\build-desktop.ps1
```

脚本会：

- 运行 `wails build`
- 把 `dist/`、`skills/`、`plugins/`、`workflows/` 一起复制到 `build/bin`
- 如果仓库根目录存在 `anyclaw.json`，也会一并复制过去

## 运行时约定

- 优先使用当前工作目录里的 `anyclaw.json`
- 如果没有，再退回到桌面版自己的用户配置目录
- `ANYCLAW_DESKTOP_CONFIG` 可以强制指定配置文件
- `ANYCLAW_DESKTOP_ROOT` 可以强制指定运行时资源根目录

## 适合当前仓库的原因

因为 AnyClaw 已经具备：

- 本地 Gateway
- 已构建的 Dashboard
- 完整的 `/providers`、`/config`、`/chat` 等接口

所以桌面壳只需要解决“原生窗口”和“启动体验”，不需要重写核心逻辑。
