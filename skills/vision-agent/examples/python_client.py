"""Small AnyClaw vision-agent REST client example.

The examples use safe demo values only. Do not type real credentials through a
desktop automation script unless the user explicitly provides them for the
current task and the target is trusted.
"""

import sys
from typing import Any

import requests


ANYCLAW_API = "http://127.0.0.1:18789"


class VisionAgent:
    def __init__(self, api_base: str = ANYCLAW_API) -> None:
        self.api = api_base.rstrip("/")
        self.screen_path = ".anyclaw/vision/screen.png"

    def run_tool(self, tool_name: str, params: dict[str, Any]) -> dict[str, Any]:
        """Execute an AnyClaw tool through the local REST API."""
        resp = requests.post(f"{self.api}/api/v1/tools/{tool_name}", json=params, timeout=30)
        resp.raise_for_status()
        return resp.json()

    def screenshot(self, path: str | None = None) -> dict[str, Any]:
        return self.run_tool("desktop_screenshot", {"path": path or self.screen_path})

    def ocr(self, path: str | None = None) -> dict[str, Any]:
        return self.run_tool("desktop_ocr", {"path": path or self.screen_path})

    def find_text(self, text: str, path: str | None = None) -> dict[str, Any]:
        return self.run_tool("desktop_find_text", {"text": text, "path": path or self.screen_path})

    def click_at(self, x: int, y: int, button: str = "left") -> dict[str, Any]:
        return self.run_tool("desktop_click", {"x": x, "y": y, "button": button})

    def click_text(self, text: str, path: str | None = None) -> dict[str, Any]:
        result = self.find_text(text, path)
        if result.get("found"):
            return self.click_at(result["center_x"], result["center_y"])
        raise RuntimeError(f"text not found: {text}")

    def type_text(self, text: str, human: bool = True) -> dict[str, Any]:
        tool = "desktop_type_human" if human else "desktop_type"
        return self.run_tool(tool, {"text": text})

    def open_app(self, app_name: str, kind: str = "app") -> dict[str, Any]:
        return self.run_tool("desktop_open", {"target": app_name, "kind": kind})

    def wait(self, ms: int) -> dict[str, Any]:
        return self.run_tool("desktop_wait", {"wait_ms": ms})

    def focus_window(self, title: str) -> dict[str, Any]:
        return self.run_tool("desktop_focus_window", {"title": title})


def example_launch_notepad() -> None:
    agent = VisionAgent()
    print("Opening Notepad...")
    agent.open_app("notepad.exe")
    agent.wait(1000)
    agent.screenshot()
    print("Captured screen after launch.")


def example_click_button() -> None:
    agent = VisionAgent()
    print("Looking for a visible Save button...")
    agent.screenshot()
    result = agent.find_text("Save")
    if not result.get("found"):
        print("Save button was not found.")
        return
    agent.click_at(result["center_x"], result["center_y"])
    print("Clicked Save.")


def example_fill_demo_form() -> None:
    agent = VisionAgent()
    print("Filling demo form fields...")
    agent.screenshot()

    username = agent.find_text("Username")
    if username.get("found"):
        agent.click_at(username["center_x"], username["center_y"])
        agent.type_text("demo-user")

    password = agent.find_text("Password")
    if password.get("found"):
        agent.click_at(password["center_x"], password["center_y"])
        agent.type_text("<credential-placeholder>")

    sign_in = agent.find_text("Sign in")
    if sign_in.get("found"):
        agent.click_at(sign_in["center_x"], sign_in["center_y"])
        print("Submitted demo form.")


def command_loop() -> None:
    agent = VisionAgent()
    print("Vision Agent demo loop. Commands: open <app>, click <text>, type <text>, quit")

    while True:
        command = input("\nCommand: ").strip()
        if command == "quit":
            return
        if command.startswith("open "):
            agent.open_app(command[5:])
            continue
        if command.startswith("click "):
            agent.click_text(command[6:])
            continue
        if command.startswith("type "):
            agent.type_text(command[5:])
            continue
        print("Unknown command")


if __name__ == "__main__":
    examples = {
        "launch_notepad": example_launch_notepad,
        "click_button": example_click_button,
        "fill_form": example_fill_demo_form,
        "loop": command_loop,
    }

    if len(sys.argv) != 2 or sys.argv[1] not in examples:
        print("Usage: python python_client.py <launch_notepad|click_button|fill_form|loop>")
        sys.exit(1)

    examples[sys.argv[1]]()
