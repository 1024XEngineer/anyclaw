# AnyClaw 灵动岛宠物最简方案

## 目标

为 AnyClaw 设计一套可以快速上线的“灵动岛宠物”方案。

这里的重点不是做完整电子宠物系统，而是把现有的桌面原子岛模式升级成一个有生命感的状态宠物层：

- 平时常驻顶部
- 用宠物动作表达 AnyClaw 当前状态
- 一键回到完整工作台
- 几乎不改后端架构

## 调研结论

灵动岛宠物之所以成立，不是因为功能多，而是因为它同时满足了 4 个条件：

1. 信息极小但一眼能懂。
2. 它始终和“正在发生的任务”绑定，而不是独立小游戏。
3. 它有轻交互，能让用户觉得“活着”，但不会打断主流程。
4. 它的反馈是情绪化的，不只是状态灯。

结合外部参考，可以提炼出对 AnyClaw 最有价值的几点：

- Apple 对 Live Activities / Dynamic Island 的设计强调内容要可扫读、紧凑、和实时活动强绑定，所以宠物层不应该再塞进完整聊天 UI。
- Pixel Pals 这类产品证明，用户愿意长期保留一个小宠物，只要它会动、会回应、会在关键时刻提示即可，不需要复杂养成也能成立。
- Dynamic Island 的交互天然适合“轻提示 + 点开即回主界面”，这和 AnyClaw 的桌面工作台结构非常匹配。

参考链接：

- Apple Human Interface Guidelines: <https://developer.apple.com/design/human-interface-guidelines/live-activities>
- Apple WWDC23, *Design dynamic Live Activities*: <https://developer.apple.com/videos/play/wwdc2023/10194/>
- Pixel Pals App Store: <https://apps.apple.com/us/app/pixel-pals-widget-pet-game/id6443919232>
- Apple Support, Dynamic Island usage: <https://support.apple.com/guide/iphone/use-and-customize-dynamic-island-iph28f50d10d/ios>

## 为什么适合 AnyClaw

AnyClaw 其实已经有“最小原子岛壳”的基础能力：

- 桌面壳已经支持 `EnterPetMode` / `ExitPetMode`
- 已经有 `PetSnapshot` 作为桌宠状态快照
- 已经通过 Gateway 的 `/status` 和 `/events` 拉取实时状态
- 前端已经有 `island-card[data-state="..."]` 的状态动画框架

也就是说，AnyClaw 缺的不是能力链路，而是“宠物化的表现层”。

因此最简单方案应该是：

保留现有架构，只替换视觉角色、动作语义和少量交互，不新开服务，不新增复杂同步链路，不做独立宠物系统。

## 最简产品定义

### 一句话定义

AnyClaw 灵动岛宠物 = 一个常驻顶部的小宠物，用动作代替状态灯，用双击代替“回到工作台”。

### V1 只做这些

- 只有 1 只默认宠物
- 只有 1 个固定尺寸岛层
- 只表达 AnyClaw 当前任务状态
- 只保留 2 个核心交互

核心交互：

- 双击宠物：展开完整 AnyClaw Desktop
- 拖动宠物：移动顶部位置

可选但仍然很轻的交互：

- 单击宠物：做一次“戳一戳”反应，不触发后端动作

### V1 明确不做

- 喂食
- 成长值
- 背包/道具
- 多宠物切换
- 宠物在桌面自由行走
- 宠物独立聊天面板
- 脱离 AnyClaw 状态的长期养成系统

## 最简单的视觉方案

### 角色建议

直接使用一只与 AnyClaw 名字一致的“爪系宠物”，建议是：

- 黑色小猫，或者
- 极简爪爪生物

原因很简单：

- 和 `AnyClaw` 命名天然一致
- 比原子/电子轨道更有情绪识别度
- 用纯 CSS 或 SVG 就能做，不需要先做复杂帧动画

### 视觉结构

宠物层继续复用当前 `320 x 92` 左右的小窗结构，只改内容布局：

- 左侧：宠物头像/半身
- 中间：状态标题
- 下方：一句短描述
- 右侧：轻提示徽标

推荐布局：

```text
[宠物] [正在思考]
       [正在整理上下文并生成回复]
```

### 动作原则

不要做逐帧动画，V1 只用 CSS / SVG 形变动作：

- 眨眼
- 歪头
- 尾巴摆动
- 小跳
- 震动
- 举爪提示

这样可以把开发量压到最低，而且后续换皮也容易。

## 状态映射

最简单做法不是重写状态系统，而是复用当前 `PetSnapshot.state`。

当前已有状态已经足够：

- `booting`
- `online`
- `thinking`
- `executing`
- `waiting`
- `complete`
- `error`
- `offline`

V1 建议把它们映射成下面的宠物表现：

| AnyClaw 状态 | 宠物表现 | 文案风格 |
| --- | --- | --- |
| `booting` | 睡醒、轻呼吸 | 正在唤醒 |
| `offline` | 趴下、变暗 | 当前离线 |
| `online` | 眨眼待机 | 随时待命 |
| `thinking` | 歪头、耳朵轻动 | 正在思考 |
| `executing` | 小跑/爪子敲击 | 正在执行 |
| `waiting` | 举爪提醒 | 等你确认 |
| `complete` | 开心跳一下 | 刚刚完成 |
| `error` | 轻晃、沮丧脸 | 出了点问题 |

关键点：

- 后端状态名先不改
- 只在前端把原来的“原子动画”换成“宠物动作”
- 文案继续沿用现有快照 detail 即可

## 交互设计

### 保留的交互

- 双击展开：继续复用当前逻辑，成本最低
- 拖动移动：继续保留当前顶部拖拽能力

### 新增但很便宜的交互

新增一个纯前端的 `poke` 交互：

- 用户单击宠物
- 宠物做一次 300 到 600ms 的反应动作
- 不调用后端
- 不影响工作流

这一步很重要，因为它能让“状态组件”变成“宠物”。

### 提示策略

只在 3 类事件上做显式提示泡泡：

- `waiting`
- `complete`
- `error`

其余状态只改动作，不弹泡泡。

原因：

- 这样最安静
- 不容易打断用户
- 和 Dynamic Island 的“轻提醒”逻辑一致

## 技术实现

### 最简架构

直接复用现有链路：

```text
Gateway /status + /events
        ->
DesktopApp.PetSnapshot()
        ->
frontend poll
        ->
根据 state 切换宠物动作和文案
```

V1 不需要：

- 新 WebSocket
- 新本地服务
- 新数据库表
- 新事件协议

### 代码落点

最小改动主要集中在这两个位置：

- `cmd/anyclaw-desktop/app.go`
- `cmd/anyclaw-desktop/frontend/src/index.html`

建议职责：

- `cmd/anyclaw-desktop/app.go`
  - 继续负责 `PetSnapshot`
  - 继续负责状态归类
  - 如有必要，只补充更宠物化的 `label/detail`

- `cmd/anyclaw-desktop/frontend/src/index.html`
  - 把原子的 DOM 改成宠物 DOM
  - 把原子的 CSS 动画改成宠物动作
  - 增加 `poke` 微交互
  - 增加等待/完成/错误的轻量气泡

### 最小改动建议

V1 连后端都可以几乎不动：

1. 保留 `PetSnapshot` 数据结构。
2. 保留现有轮询节奏。
3. 保留 `derivePetState()` 的状态推导。
4. 只替换前端视觉实现。

如果还想再前进一步，但仍然保持简单，可以额外做一个小改动：

5. 在 `derivePetState()` 中补一个更稳定的“宠物短语”，避免前端自己拼文案。

## 推荐实施顺序

### 第 1 步：先把“原子岛”变成“宠物岛”

只改前端视觉层：

- 原子轨道改成宠物
- 保留状态字段不动
- 保留双击展开

这是最低风险、见效最快的一步。

### 第 2 步：补 3 个关键提示

只对以下状态增加气泡提示：

- 等待确认
- 完成
- 失败

做到这里，用户已经会明显感觉它是“活的”。

### 第 3 步：补一个轻交互

加入单击 `poke` 反应动作。

做到这里，AnyClaw 的灵动岛宠物就已经成立了。

## 这套方案的优点

- 几乎完全复用现有桌面壳架构
- 不会把桌宠做成独立系统，复杂度可控
- 和 AnyClaw 的实时任务状态天然绑定
- 很适合先上线，再慢慢长出主题、音效和更多角色

## 后续扩展方向

如果 V1 跑通，再考虑这些增强项：

- 宠物主题切换
- 不同模型/不同代理对应不同表情
- 审批等待时的更强提醒
- 语音唤醒时的嘴型/耳朵反馈
- 宠物泡泡里显示最近一次工具名

但这些都应该放在 V1 之后。

## 最终建议

对 AnyClaw 来说，最正确的最简方案不是“做一个完整宠物游戏”，而是：

先把现有原子岛改造成一个有情绪、有状态、有轻交互的 AnyClaw 岛宠。

一句话版本：

保留当前 `PetSnapshot + Wails 小窗 + 轮询状态` 架构，只把原子动画替换成宠物动作，并增加 `poke` 和关键事件气泡，这就是适合 AnyClaw 的最简单灵动岛宠物方案。
