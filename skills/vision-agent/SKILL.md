# Vision Agent Skill

This skill enables Siri/Cortana/Xiaoyuanzi-like capabilities - an AI agent that can "see" the screen and autonomously control desktop applications.

## Overview

The Vision Agent is an autonomous desktop control system that:
1. **Captures** - Takes screenshots of the screen
2. **Understands** - Uses AI/Vision to understand what's on screen
3. **Decides** - Determines what action to take based on user intent
4. **Acts** - Executes clicks, typing, hotkeys, etc.
5. **Verifies** - Confirms the action succeeded

## How It Works

```
User Command (Voice/Text)
        │
        ▼
┌───────────────────┐
│  Vision Agent     │
│  ┌─────────────┐  │
│  │ 1. Screenshot│  │
│  └─────────────┘  │
│         │         │
│  ┌─────────────┐  │
│  │ 2. OCR/Vision│ │
│  │    Analyze   │  │
│  └─────────────┘  │
│         │         │
│  ┌─────────────┐  │
│  │ 3. Find Target│ │
│  │ (text/image) │  │
│  └─────────────┘  │
│         │         │
│  ┌─────────────┐  │
│  │ 4. Execute  │  │
│  │ (click/type)│  │
│  └─────────────┘  │
│         │         │
│  ┌─────────────┐  │
│  │ 5. Verify   │  │
│  │ (screenshot) │  │
│  └─────────────┘  │
└───────────────────┘
        │
        ▼
  Action Result
```

## Capabilities

### 1. Screen Understanding
- **OCR**: Read all text on screen
- **Element Query**: Find UI elements (buttons, inputs, etc.)
- **Image Match**: Locate icons/images

### 2. Visual Actions
- **Click**: By coordinates, text, or image
- **Type**: Text input with human-like delays
- **Hotkey**: Keyboard shortcuts
- **Drag**: Drag and drop operations
- **Scroll**: Scroll up/down

### 3. Window Management
- Focus/minimize/maximize/close windows
- Launch applications
- Screenshot capture

### 4. Smart Automation
- Multi-step task execution
- Retry on failure
- Result verification

## Available Tools

| Tool | Description |
|------|-------------|
| `vision_screenshot` | Capture screen for analysis |
| `vision_ocr` | Extract text from screen |
| `vision_find_text` | Locate text position |
| `vision_click_text` | Click on text location |
| `vision_match_image` | Find image on screen |
| `vision_click_image` | Click on matched image |
| `vision_query_ui` | Query UI automation elements |
| `vision_execute` | Execute a sequence of actions |

## Usage Patterns

### Pattern 1: Direct Command
```
User: "打开微信"
Agent:
  1. desktop_open with app="wechat"
  2. wait for app to launch
  3. focus window
  4. verify success
```

### Pattern 2: Visual Navigation
```
User: "点击发送按钮"
Agent:
  1. screenshot
  2. ocr to find "发送"
  3. click at text coordinates
  4. verify button changed state
```

### Pattern 3: Multi-Step Task
```
User: "在微信里给张三发消息说我马上到"
Agent:
  1. open WeChat
  2. find and click Zhang San's chat
  3. find input box
  4. type "我马上到"
  5. find and click send button
  6. verify message sent
```

## Workflow Examples

### Example 1: Launch and Use App
```json
{
  "task": "打开记事本并输入Hello World",
  "steps": [
    {"action": "desktop_open", "target": "notepad.exe"},
    {"action": "desktop_wait", "ms": 1000},
    {"action": "desktop_focus_window", "title": "记事本"},
    {"action": "desktop_type_human", "text": "Hello World"}
  ]
}
```

### Example 2: Click Button by Text
```json
{
  "task": "点击确定按钮",
  "steps": [
    {"action": "screenshot"},
    {"action": "find_text", "text": "确定"},
    {"action": "click", "at": "found_text_center"}
  ]
}
```

### Example 3: Form Filling
```json
{
  "task": "在网页登录表单输入账号密码并登录",
  "steps": [
    {"action": "screenshot"},
    {"action": "find_text", "text": "用户名"},
    {"action": "click"},
    {"action": "desktop_type", "text": "myaccount"},
    {"action": "find_text", "text": "密码"},
    {"action": "click"},
    {"action": "desktop_type", "text": "mypassword"},
    {"action": "find_text", "text": "登录"},
    {"action": "click"}
  ]
}
```

## Configuration

In `anyclaw.json`:

```json
{
  "sandbox": {
    "enabled": true,
    "execution_mode": "host-reviewed"
  },
  "agent": {
    "permission_level": "full"
  }
}
```

## Best Practices

1. **Always verify**: Take screenshot after actions to confirm success
2. **Add delays**: Wait for UI to respond between actions
3. **Human-like**: Use `desktop_type_human` for text input
4. **Retry logic**: If action fails, retry with slight delay
5. **Clear targets**: Be specific in commands ("点击绿色按钮" vs "点击按钮")

## Error Handling

The agent handles common issues:
- Element not found → retry or fallback
- Wrong window focused → refocus and retry
- Action timeout → wait and retry
- Verification failed → try alternative approach
