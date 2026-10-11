"""Print only Active Directory sync status text. No credentials."""
import sys
sys.path.insert(0, ".")
from db_config import _CONFIG
from live_plugins_section_test import make_client, api_call

PASSWORD = _CONFIG.get("ADMIN_PASSWORD", "")

def main():
    client = make_client()
    status, _, _ = api_call(client, "POST", "/api/session/login", {"username": "admin", "password": PASSWORD})
    if status != 200:
        print("LOGIN", status)
        return
    _, body, _ = api_call(client, "GET", "/api/scim/ad/config")
    if not isinstance(body, dict):
        print("AD_BAD")
        return
    msg = str(body.get("last_sync_message") or "")
    print("status", body.get("last_sync_status"))
    print("at", body.get("last_sync_at"))
    print("count", body.get("last_sync_count"))
    print("discovered", body.get("discovered_users_count"))
    print("message", msg[:400])
    oauth = body.get("oauth")
    cfg_oauth = None
    _, scim, _ = api_call(client, "GET", "/api/scim/config")
    if isinstance(scim, dict) and isinstance(scim.get("config"), dict):
        oauth = scim["config"].get("oauth")
    if isinstance(oauth, dict):
        print("oauth_keys", sorted(oauth.keys()))
        print("oauth_connected", bool(oauth.get("access_token") or oauth.get("refresh_token") or oauth.get("connected")))
    else:
        print("oauth_type", type(oauth).__name__)

if __name__ == "__main__":
    main()
