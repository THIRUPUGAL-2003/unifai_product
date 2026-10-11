import os
import time
import shutil
import subprocess
import urllib.request
from pathlib import Path

TARGET_DIR = Path(os.environ["LOCALAPPDATA"]) / "Programs" / "Gateway" / "Guard"
SRC_EXE = Path(r"D:\unifai_project\apps\browser-guard\build\exe\Gateway_Guard.exe")
SRC_CONFIG = Path(r"D:\unifai_project\apps\browser-guard\release\gateway_guard_config.json")

print(f"Target dir: {TARGET_DIR}")
print(f"Source EXE: {SRC_EXE} (size={SRC_EXE.stat().st_size})")

# 1. Kill old Gateway_Guard processes
print("Stopping running Gateway_Guard processes...")
subprocess.run(["taskkill", "/F", "/IM", "Gateway_Guard.exe"], capture_output=True)
time.sleep(2)

# 2. Copy new binary
dest_exe = TARGET_DIR / "Gateway_Guard.exe"
print(f"Replacing {dest_exe} with new binary...")
shutil.copy2(SRC_EXE, dest_exe)
if SRC_CONFIG.is_file():
    shutil.copy2(SRC_CONFIG, TARGET_DIR / "gateway_guard_config.json")
print(f"Successfully copied new Gateway_Guard.exe! Size: {dest_exe.stat().st_size}")

# 3. Start new Gateway_Guard.exe
print("Starting new Gateway_Guard.exe...")
p = subprocess.Popen([str(dest_exe)], cwd=str(TARGET_DIR), creationflags=subprocess.CREATE_NO_WINDOW if hasattr(subprocess, "CREATE_NO_WINDOW") else 0)
print(f"Spawned new Gateway_Guard with PID {p.pid}")

# 4. Wait and verify port 18103
print("Waiting for proxy port 18103...")
active = False
for i in range(15):
    time.sleep(1)
    try:
        req = urllib.request.Request("http://127.0.0.1:18103/healthz", headers={"Host": "gateway.internal"})
        with urllib.request.urlopen(req, timeout=2) as resp:
            print(f"Healthcheck OK: status={resp.status}")
            active = True
            break
    except Exception as e:
        # Check standard proxy ping
        try:
            proxy_support = urllib.request.ProxyHandler({'http': 'http://127.0.0.1:18103'})
            opener = urllib.request.build_opener(proxy_support)
            req = urllib.request.Request("http://example.com")
            with opener.open(req, timeout=2) as resp:
                print(f"Proxy responding: status={resp.status}")
                active = True
                break
        except Exception:
            pass

print(f"Proxy active: {active}")
