"""Turn telemetry on, confirm it is loaded, then turn it back off."""
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
    before = body.get("plugins") if isinstance(body, dict) else []
    print("LOADED_BEFORE", "telemetry" in before, before)

    st, body, _ = api_call(client, "PUT", "/api/plugins/telemetry", {"enabled": True, "config": {}})
    msg = ""
    if isinstance(body, dict):
        msg = body.get("message") or body.get("error") or ""
        plug = body.get("plugin") or {}
        print("ENABLE", st, msg, "enabled", plug.get("enabled"), "runtime", (plug.get("status") or {}).get("status"))
    else:
        print("ENABLE", st, brief(body))

    st, body, _ = api_call(client, "GET", "/api/plugins/loaded")
    after = body.get("plugins") if isinstance(body, dict) else []
    print("LOADED_AFTER_ENABLE", "telemetry" in after)

    st, body, _ = api_call(client, "PUT", "/api/plugins/telemetry", {"enabled": False, "config": {}})
    msg = body.get("message") or body.get("error") if isinstance(body, dict) else brief(body)
    plug = body.get("plugin") if isinstance(body, dict) else {}
    print("DISABLE", st, msg, "enabled", (plug or {}).get("enabled"), "runtime", ((plug or {}).get("status") or {}).get("status"))

    st, body, _ = api_call(client, "GET", "/api/plugins/loaded")
    final = body.get("plugins") if isinstance(body, dict) else []
    print("LOADED_AFTER_DISABLE", "telemetry" in final)

    st, body, _ = api_call(client, "GET", "/api/plugins")
    names = []
    if isinstance(body, dict):
        for p in body.get("plugins") or []:
            if p.get("name") in ("telemetry", "semantic_cache", "governance"):
                names.append((p.get("name"), p.get("enabled"), p.get("isCustom"), (p.get("status") or {}).get("status")))
    print("ROWS", names)

if __name__ == "__main__":
    main()
