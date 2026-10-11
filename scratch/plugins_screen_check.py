"""Compare the Plugins page dots with what is actually loaded. No secrets."""
import sys
sys.path.insert(0, ".")
from db_config import _CONFIG
from live_plugins_section_test import make_client, api_call, brief

PASSWORD = _CONFIG.get("ADMIN_PASSWORD", "")

def main():
    client = make_client()
    status, resp, _ = api_call(client, "POST", "/api/session/login", {"username": "admin", "password": PASSWORD})
    print("LOGIN", status, "ok" if status == 200 else brief(resp))
    if status != 200:
        return
    st, body, _ = api_call(client, "GET", "/api/plugins/loaded")
    loaded = body.get("plugins") if isinstance(body, dict) else []
    print("LOADED", loaded)
    st, body, _ = api_call(client, "GET", "/api/plugins")
    if isinstance(body, dict):
        for p in body.get("plugins") or []:
            print(p.get("name"), "db_enabled", p.get("enabled"), "custom", p.get("isCustom"), "runtime", (p.get("status") or {}).get("status"))

if __name__ == "__main__":
    main()
