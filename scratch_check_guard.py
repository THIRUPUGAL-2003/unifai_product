import subprocess
import socket
import json
import urllib.request
import os

def check_port(port):
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(1.0)
        return s.connect_ex(('127.0.0.1', port)) == 0

from pathlib import Path
env_file = Path(__file__).resolve().parent / ".env"
env_vars = {}
if env_file.is_file():
    for line in env_file.read_text(encoding="utf-8").splitlines():
        if "=" in line and not line.strip().startswith("#"):
            k, v = line.split("=", 1)
            env_vars[k.strip()] = v.strip().strip("'\"")

app_port = int(env_vars.get("APP_PORT") or os.environ.get("APP_PORT") or 0)
pac_port = int(env_vars.get("PAC_HTTP_PORT") or os.environ.get("PAC_HTTP_PORT") or 0)
proxy_addr = env_vars.get("GATEWAY_PROXY_ADDR") or os.environ.get("GATEWAY_PROXY_ADDR") or ""
proxy_port = int(proxy_addr.split(":")[-1]) if ":" in proxy_addr else 0
if not app_port or not pac_port or not proxy_port:
    raise ValueError(f"Missing required ports in .env: APP_PORT={app_port}, PAC_HTTP_PORT={pac_port}, GATEWAY_PROXY_ADDR={proxy_addr}")

print(f"Port {proxy_port} (mitmproxy):", check_port(proxy_port))
print(f"Port {pac_port} (PAC):", check_port(pac_port))
print(f"Port {app_port} (backend):", check_port(app_port))

# Check processes
cmd = 'tasklist /fi "imagename eq Gateway_Guard.exe"'
res = subprocess.run(cmd, capture_output=True, text=True, shell=True)
print("Processes:")
print(res.stdout)
