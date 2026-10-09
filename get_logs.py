import urllib.request
import json

headers = {
    'Accept': 'application/json',
    'X-Gateway-Guard-Key': 'ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo='
}
req = urllib.request.Request('http://127.0.0.1:8080/api/browser-ai/logs?limit=25', headers=headers)
try:
    with urllib.request.urlopen(req, timeout=3) as resp:
        data = json.loads(resp.read().decode())
        logs = data.get('logs', [])
        print(f"Logs count: {len(logs)}")
        for l in logs:
            plat = l.get("platform")
            dom = l.get("domain")
            stat = l.get("status")
            blk = l.get("is_blocked")
            p = l.get("prompt", "") or ""
            r = l.get("rule_triggered", "") or ""
            ts = l.get("created_at", "")
            print(f"[{plat}] {dom} | status={stat} blk={blk} rule={r} | prompt={p[:70]!r} | {ts}")
except Exception as e:
    print('Error:', e)
