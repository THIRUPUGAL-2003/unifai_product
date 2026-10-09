import urllib.request
import json

headers = {
    "Accept": "application/json",
    "X-Gateway-Guard-Key": "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="
}
req = urllib.request.Request("http://127.0.0.1:8080/api/browser-ai/targets", headers=headers)
with urllib.request.urlopen(req, timeout=3) as resp:
    data = json.loads(resp.read().decode())
    targets = data.get("targets", [])
    print(f"Total targets: {len(targets)}")
    for t in targets:
        domain = t.get("domain", "")
        platform = t.get("platform_name", "")
        monitored = t.get("monitored")
        block_site = t.get("block_site")
        role = t.get("host_role", "")
        parent_id = t.get("parent_id", "")
        print(f"[{domain}] platform={platform} monitored={monitored} block_site={block_site} role={role} parent={parent_id}")
