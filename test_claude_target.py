import sys
import os
import json

_HERE = os.path.dirname(os.path.abspath(__file__))
_PARTS_DIR = os.path.join(_HERE, "apps", "browser-guard", "proxy", "gateway_proxy_parts")

manifest = os.path.join(_PARTS_DIR, "MANIFEST.txt")
with open(manifest, encoding="utf-8-sig") as f:
    names = [ln.strip() for ln in f if ln.strip()]
ns = {}
for name in names:
    path = os.path.join(_PARTS_DIR, name)
    with open(path, encoding="utf-8") as f:
        code = compile(f.read(), name, "exec")
        exec(code, ns)

print("Loaded proxy parts. Total names in namespace:", len(ns))

# Let's inspect what detect_target returns for claude.ai and subdomains:
detect_target = ns["detect_target"]
for host in ["claude.ai", "api.anthropic.com", "cdn.claude.ai", "files.claudeusercontent.com", "anthropic.com"]:
    print(f"detect_target({host}) ->", detect_target(host))

# Let's check get_target_host_role
get_target_host_role = ns["get_target_host_role"]
for host in ["claude.ai", "api.anthropic.com", "cdn.claude.ai", "files.claudeusercontent.com"]:
    print(f"get_target_host_role({host}) ->", get_target_host_role(host))
