"""Check User Provisioning connections. Never print tokens or passwords."""
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

    st, body, _ = api_call(client, "GET", "/api/scim/config")
    if isinstance(body, dict):
        cfg = body.get("config") if isinstance(body.get("config"), dict) else {}
        print(
            "SCIM", st,
            "enabled", body.get("enabled"),
            "provider", body.get("provider"),
            "token_set", bool(body.get("bearer_token")),
            "config_keys", sorted(cfg.keys()),
        )
    else:
        print("SCIM", st, brief(body, 180))

    st, body, _ = api_call(client, "GET", "/api/scim/providers")
    print("PROVIDERS", st, brief(body, 200))

    st, body, _ = api_call(client, "GET", "/api/scim/ad/config")
    if isinstance(body, dict):
        print(
            "AD", st,
            "enabled", body.get("enabled"),
            "server_set", bool(body.get("server_url")),
            "domain_set", bool(body.get("domain")),
            "base_dn_set", bool(body.get("base_dn")),
            "bind_dn_set", bool(body.get("bind_dn")),
            "password_set", bool(body.get("bind_password")),
            "last_sync_status", body.get("last_sync_status"),
            "last_sync_count", body.get("last_sync_count"),
        )
    else:
        print("AD", st, brief(body, 180))

    st, body, _ = api_call(client, "GET", "/scim/v2/ServiceProviderConfig")
    print("SCIM_PUBLIC", st, brief(body, 160))

if __name__ == "__main__":
    main()
