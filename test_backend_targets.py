import urllib.request
import json

def main():
    endpoints = [
        "http://127.0.0.1:8080/api/browser-ai/targets",
        "http://127.0.0.1:8080/api/browser-ai/rules",
        "http://127.0.0.1:8080/api/browser-ai/controls",
    ]
    for ep in endpoints:
        print(f"\n=== GET {ep} ===")
        try:
            headers = {
                "Accept": "application/json",
                "X-Gateway-Guard-Key": "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="
            }
            req = urllib.request.Request(ep, headers=headers)
            with urllib.request.urlopen(req, timeout=3) as resp:
                data = json.loads(resp.read().decode())
                if isinstance(data, list):
                    print(f"Count: {len(data)}")
                    for item in data[:30]:
                        if "domain" in item:
                            print(f"  Target: domain={item.get('domain')} platform={item.get('platform')} active={item.get('is_active')}")
                        elif "pattern" in item or "rule" in str(item):
                            print(f"  Rule: name={item.get('name')} action={item.get('action')} pattern={item.get('pattern')} active={item.get('is_active')}")
                        else:
                            print("  Item:", item)
                elif isinstance(data, dict):
                    print("Dict:", json.dumps(data, indent=2)[:300])
        except Exception as e:
            print(f"Error: {e}")

if __name__ == "__main__":
    main()
