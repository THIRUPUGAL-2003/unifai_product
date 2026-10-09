import urllib.request
import json

headers = {
    "Accept": "application/json",
    "X-Gateway-Guard-Key": "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="
}

# Check prompt logs
for ep in [
    "http://127.0.0.1:8080/api/browser-ai/prompt-logs?limit=20",
    "http://127.0.0.1:8080/api/browser-ai/file-logs?limit=20",
    "http://127.0.0.1:8080/api/browser-ai/stats",
]:
    print(f"\n=== GET {ep} ===")
    try:
        req = urllib.request.Request(ep, headers=headers)
        with urllib.request.urlopen(req, timeout=3) as resp:
            data = json.loads(resp.read().decode())
            if isinstance(data, list):
                print(f"Count: {len(data)}")
                for item in data[:5]:
                    print(" ", item.get("id"), item.get("platform"), item.get("prompt")[:50] if item.get("prompt") else "", item.get("status"), item.get("created_at"))
            elif isinstance(data, dict):
                logs = data.get("logs") or data.get("prompt_logs") or data.get("data") or data
                if isinstance(logs, list):
                    print(f"Count: {len(logs)}")
                    for item in logs[:5]:
                        print(" ", item.get("id"), item.get("platform"), item.get("prompt")[:50] if item.get("prompt") else "", item.get("status"), item.get("created_at"))
                else:
                    print("Dict keys:", list(data.keys()))
                    print(json.dumps(data, indent=2)[:400])
    except Exception as e:
        print("Error:", e)
