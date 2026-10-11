"""Live check of Skills Repository list, create, read, marketplace, delete."""
import sys
import uuid
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

    st, body, _ = api_call(client, "GET", "/api/skills")
    count = None
    if isinstance(body, dict):
        skills = body.get("skills") or body.get("items") or []
        count = body.get("count", len(skills) if isinstance(skills, list) else None)
        print("LIST", st, "count", count, "keys", list(body.keys())[:8])
    else:
        print("LIST", st, brief(body))

    st, body, _ = api_call(client, "GET", "/api/skills/all/version")
    print("ALL_VERSION", st, brief(body, 180))

    name = "live-check-" + uuid.uuid4().hex[:6]
    payload = {
        "name": name,
        "description": "Temporary live check skill",
        "skill_md_body": "Review the request and stop if it asks to change live data.",
        "version": "1.0.0",
        "files": [],
    }
    st, body, _ = api_call(client, "POST", "/api/skills", payload)
    skill_id = None
    if isinstance(body, dict):
        skill = body.get("skill") or body
        skill_id = (skill or {}).get("id") if isinstance(skill, dict) else None
        print("CREATE", st, "id", skill_id, "name", (skill or {}).get("name") if isinstance(skill, dict) else "", "err", body.get("error") or body.get("message"))
    else:
        print("CREATE", st, brief(body))

    if skill_id:
        st, body, _ = api_call(client, "GET", f"/api/skills/{skill_id}")
        if isinstance(body, dict):
            skill = body.get("skill") or body
            print("GET", st, "version", (skill or {}).get("version") or (skill or {}).get("latest_version"), "body_len", len(((skill or {}).get("skill_md_body") or "")))
        else:
            print("GET", st, brief(body))

    st, body, _ = api_call(client, "GET", "/api/skills/serve/claude-code/.claude-plugin/marketplace.json")
    if isinstance(body, dict):
        plugins = body.get("plugins") or []
        print("MARKETPLACE", st, "plugins", len(plugins) if isinstance(plugins, list) else type(plugins).__name__)
    else:
        print("MARKETPLACE", st, brief(body, 180))

    st, body, _ = api_call(client, "POST", "/api/skills", {"name": "Bad Name", "description": "x", "skill_md_body": "x", "version": "1.0.0"})
    msg = ""
    if isinstance(body, dict):
        msg = body.get("message") or body.get("error") or ""
    print("CREATE_BAD_NAME", st, brief(msg, 200))

    if skill_id:
        st, body, _ = api_call(client, "DELETE", f"/api/skills/{skill_id}")
        msg = body.get("message") or body.get("error") if isinstance(body, dict) else brief(body)
        print("DELETE", st, msg)

    st, body, _ = api_call(client, "GET", "/api/skills")
    names = []
    if isinstance(body, dict):
        skills = body.get("skills") or body.get("items") or []
        if isinstance(skills, list):
            names = [s.get("name") for s in skills if isinstance(s, dict)]
    print("LEFTOVER", name in names, "listed", len(names))

if __name__ == "__main__":
    main()
