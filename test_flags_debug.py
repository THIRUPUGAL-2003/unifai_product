import urllib.request, json, ssl, sys
sys.stdout.reconfigure(encoding='utf-8')

import os
from pathlib import Path
env_file = Path(__file__).resolve().parent / ".env"
env_vars = {}
if env_file.is_file():
    for line in env_file.read_text(encoding="utf-8").splitlines():
        if "=" in line and not line.strip().startswith("#"):
            k, v = line.split("=", 1)
            env_vars[k.strip()] = v.strip().strip("'\"")

backend_url = env_vars.get("SERVER_DOMAIN") or env_vars.get("GATEWAY_BACKEND_URL") or (f"http://127.0.0.1:{env_vars.get('APP_PORT')}" if env_vars.get('APP_PORT') else "")
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

admin_user = env_vars.get("ADMIN_EMAIL", "")
admin_pass = env_vars.get("ADMIN_PASSWORD", "")
login_data = json.dumps({"username": admin_user, "password": admin_pass}).encode('utf-8')
req = urllib.request.Request(f"{backend_url}/api/session/login", data=login_data, headers={"Content-Type": "application/json"}, method="POST")

with urllib.request.urlopen(req, context=ctx) as resp:
    cookies = resp.headers.get("Set-Cookie", "")
    token = [p.split("=")[1] for p in cookies.split(";") if "token=" in p][0]

headers = {
    "Authorization": f"Bearer {token}",
    "Cookie": f"token={token}",
    "Content-Type": "application/json"
}

req = urllib.request.Request(f"{backend_url}/api/feature-flags", headers=headers, method="GET")
with urllib.request.urlopen(req, context=ctx) as resp:
    data = json.loads(resp.read().decode('utf-8'))
    print(f"Total flags returned: {len(data['flags'])}")
    for f in data['flags']:
        print(f" - {f['id']} | {f.get('display_name')} | registered: {f.get('registered')}")
