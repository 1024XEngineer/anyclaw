# Vision Agent Skill

This skill helps AnyClaw use desktop vision and UI automation tools safely.
It is intended for host-reviewed desktop workflows where the agent must observe,
act, and verify each step.

## Overview

The vision agent loop is:

1. Capture the current screen or target window.
2. Read visible text or inspect UI targets.
3. Choose the smallest safe action.
4. Execute the action with desktop tools.
5. Verify the result before continuing.

## Capabilities

- Capture screenshots for visual inspection.
- Extract visible text with OCR.
- Locate text, buttons, fields, windows, and app targets.
- Click, type, use hotkeys, scroll, and focus windows.
- Chain actions into a reviewed desktop workflow.
- Verify meaningful state changes after each action.

## Recommended Tool Order

1. Use `desktop_plan` for multi-step desktop tasks.
2. Use `desktop_resolve_target` and `desktop_activate_target` for stable UI targets.
3. Use `desktop_set_target_value` for form fields when available.
4. Fall back to `desktop_screenshot`, `desktop_ocr`, and `desktop_find_text` when needed.
5. Use `desktop_verify_text` or a fresh screenshot before reporting completion.

## Usage Patterns

### Launch And Verify An App

```json
{
  "task": "Open Notepad and verify it is ready",
  "steps": [
    {"action": "desktop_open", "target": "notepad.exe"},
    {"action": "desktop_wait", "ms": 1000},
    {"action": "desktop_focus_window", "title": "Notepad"},
    {"action": "desktop_verify_text", "expected": "Untitled"}
  ]
}
```

### Click A Button By Visible Text

```json
{
  "task": "Click the Save button",
  "steps": [
    {"action": "desktop_screenshot"},
    {"action": "desktop_find_text", "text": "Save"},
    {"action": "desktop_click", "at": "found_text_center"}
  ]
}
```

### Fill A Form Safely

```json
{
  "task": "Fill a demo form",
  "steps": [
    {"action": "desktop_screenshot"},
    {"action": "desktop_find_text", "text": "Username"},
    {"action": "desktop_click"},
    {"action": "desktop_type_human", "text": "demo-user"},
    {"action": "desktop_find_text", "text": "Password"},
    {"action": "desktop_click"},
    {"action": "desktop_type_human", "text": "<credential-placeholder>"},
    {"action": "desktop_find_text", "text": "Sign in"},
    {"action": "desktop_click"}
  ]
}
```

## Safety Notes

- Do not type credentials unless the user explicitly provides them for the current task.
- Prefer semantic targets over raw coordinates when possible.
- Keep actions reversible when a UI state is uncertain.
- Stop and ask before destructive or externally visible actions.
- Verify results with a fresh observation before claiming success.
