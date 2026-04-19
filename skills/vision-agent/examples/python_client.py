# Vision Agent - Python Client Example
# This script demonstrates how to use AnyClaw's vision-agent to control desktop apps

import json
import time
import subprocess
import requests

# Configuration
ANYCLAW_API = "http://127.0.0.1:18789"
WS_URL = "ws://127.0.0.1:18789/ws"

class VisionAgent:
    def __init__(self, api_base=ANYCLAW_API):
        self.api = api_base
        self.screen_path = ".anyclaw/vision/screen.png"
        
    def run_tool(self, tool_name, params):
        """Execute an AnyClaw tool via REST API"""
        resp = requests.post(
            f"{self.api}/api/v1/tools/{tool_name}",
            json=params
        )
        return resp.json()
    
    def screenshot(self, path=None):
        """Take a screenshot"""
        p = path or self.screen_path
        return self.run_tool("desktop_screenshot", {"path": p})
    
    def ocr(self, path=None):
        """Extract text from screenshot"""
        p = path or self.screen_path
        return self.run_tool("desktop_ocr", {"path": p})
    
    def find_text(self, text, path=None):
        """Find text position on screen"""
        p = path or self.screen_path
        result = self.run_tool("desktop_find_text", {
            "text": text,
            "path": p
        })
        return result
    
    def click_at(self, x, y, button="left"):
        """Click at coordinates"""
        return self.run_tool("desktop_click", {
            "x": x, "y": y, "button": button
        })
    
    def click_text(self, text, path=None):
        """Find and click on text"""
        result = self.find_text(text, path)
        if result.get("found"):
            return self.click_at(result["center_x"], result["center_y"])
        raise Exception(f"Text '{text}' not found")
    
    def type_text(self, text, human=True):
        """Type text"""
        if human:
            return self.run_tool("desktop_type_human", {"text": text})
        return self.run_tool("desktop_type", {"text": text})
    
    def open_app(self, app_name, kind="app"):
        """Open an application"""
        return self.run_tool("desktop_open", {
            "target": app_name,
            "kind": kind
        })
    
    def wait(self, ms):
        """Wait for specified milliseconds"""
        return self.run_tool("desktop_wait", {"wait_ms": ms})
    
    def focus_window(self, title):
        """Focus window by title"""
        return self.run_tool("desktop_focus_window", {"title": title})


def example_launch_wechat():
    """Example: Launch WeChat"""
    agent = VisionAgent()
    
    print("1. Opening WeChat...")
    agent.open_app("wechat")
    
    print("2. Waiting for app to launch...")
    agent.wait(3000)
    
    print("3. Taking screenshot...")
    agent.screenshot()
    
    print("4. OCR analyzing screen...")
    text = agent.ocr()
    print(f"   Found text: {text.get('text', '')[:100]}...")
    
    print("5. Verifying WeChat is open...")
    result = agent.find_text("微信")
    if result.get("found"):
        print("   ✓ WeChat launched successfully!")
    else:
        print("   ✗ Could not verify launch")


def example_click_button():
    """Example: Click a button by text"""
    agent = VisionAgent()
    
    print("1. Taking screenshot...")
    agent.screenshot()
    
    print("2. Finding '确定' button...")
    result = agent.find_text("确定")
    
    if result.get("found"):
        print(f"   Found at ({result['x']}, {result['y']})")
        print("3. Clicking button...")
        agent.click_at(result["center_x"], result["center_y"])
        print("   ✓ Button clicked!")
    else:
        print("   ✗ Button not found")


def example_send_wechat_message():
    """Example: Send a message in WeChat"""
    agent = VisionAgent()
    
    # Step 1: Open WeChat
    print("1. Opening WeChat...")
    agent.open_app("wechat")
    agent.wait(3000)
    agent.focus_window("微信")
    
    # Step 2: Find contact
    print("2. Finding contact '张三'...")
    agent.screenshot()
    result = agent.find_text("张三")
    if not result.get("found"):
        print("   Contact not found, trying alternative...")
        result = agent.find_text("文件传输助手")
    
    if result.get("found"):
        agent.click_at(result["center_x"], result["center_y"])
        agent.wait(1000)
        
        # Step 3: Type message
        print("3. Typing message...")
        agent.type_text("我马上到")
        
        # Step 4: Click send
        print("4. Clicking send button...")
        result = agent.find_text("发送")
        if result.get("found"):
            agent.click_at(result["center_x"], result["center_y"])
            print("   ✓ Message sent!")
        else:
            # Try pressing Enter instead
            agent.run_tool("desktop_hotkey", {"keys": ["enter"]})
    else:
        print("   ✗ Could not find chat")


def example_fill_form():
    """Example: Fill a login form"""
    agent = VisionAgent()
    
    print("1. Taking initial screenshot...")
    agent.screenshot()
    
    # Fill username
    print("2. Finding username field...")
    result = agent.find_text("用户名")
    if result.get("found"):
        agent.click_at(result["center_x"], result["center_y"])
        agent.type_text("admin")
    
    # Fill password
    print("3. Finding password field...")
    result = agent.find_text("密码")
    if result.get("found"):
        agent.click_at(result["center_x"], result["center_y"])
        agent.type_text("mypassword")
    
    # Click login
    print("4. Clicking login button...")
    result = agent.find_text("登录")
    if result.get("found"):
        agent.click_at(result["center_x"], result["center_y"])
        print("   ✓ Form submitted!")


def voice_command_loop():
    """Example: Continuous voice command loop (requires external STT)"""
    agent = VisionAgent()
    
    print("Vision Agent started. Waiting for commands...")
    print("Commands: open <app>, click <text>, type <text>, send <message>")
    
    while True:
        # This would connect to your voice input system
        # For now, we simulate with input()
        command = input("\nEnter command (or 'quit'): ").strip()
        
        if command == "quit":
            break
        elif command.startswith("open "):
            app = command[5:]
            agent.open_app(app)
        elif command.startswith("click "):
            text = command[6:]
            try:
                agent.click_text(text)
                print(f"Clicked on '{text}'")
            except Exception as e:
                print(f"Error: {e}")
        elif command.startswith("type "):
            text = command[5:]
            agent.type_text(text)
        else:
            print("Unknown command")


if __name__ == "__main__":
    import sys
    
    if len(sys.argv) < 2:
        print("Usage: python vision_agent_client.py <example>")
        print("Examples:")
        print("  launch_wechat   - Launch WeChat")
        print("  click_button    - Click a button by text")
        print("  send_message    - Send a WeChat message")
        print("  fill_form       - Fill a login form")
        print("  voice           - Voice command loop")
        sys.exit(1)
    
    example = sys.argv[1]
    
    examples = {
        "launch_wechat": example_launch_wechat,
        "click_button": example_click_button,
        "send_message": example_send_wechat_message,
        "fill_form": example_fill_form,
        "voice": voice_command_loop,
    }
    
    if example in examples:
        examples[example]()
    else:
        print(f"Unknown example: {example}")
