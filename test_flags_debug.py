import urllib.request, json, ssl, sys
sys.stdout.reconfigure(encoding='utf-8')

backend_url = "http://localhost:8080"
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

login_data = json.dumps({"username": "admin@yespanchi.com", "password": "YP2025-2026yp"}).encode('utf-8')
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
