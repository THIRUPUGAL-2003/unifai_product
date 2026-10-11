"""Live test of the Plugins sidebar section. Prints statuses only, no secrets."""
import json
import sys
import uuid
import http.cookiejar
import urllib.request
import urllib.error

sys.path.insert(0, ".")
from db_config import get_base_url, _CONFIG

BASE = get_base_url()
EMAIL = _CONFIG.get("ADMIN_EMAIL", "")
PASSWORD = _CONFIG.get("ADMIN_PASSWORD", "")

def make_client():
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    return opener

def api_call(opener, method, path, body=None):
    url = f"{BASE}{path}"
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method=method)
    try:
        with opener.open(req) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
            try:
                parsed = json.loads(raw)
            except Exception:
                parsed = raw
            return resp.status, parsed, None
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace")
        try:
            parsed_err = json.loads(err_body)
        except Exception:
            parsed_err = err_body
        return e.code, parsed_err, str(e)
    except Exception as e:
        return 0, None, str(e)

def brief(obj, limit=400):
    text = obj if isinstance(obj, str) else json.dumps(obj, default=str)
    # strip likely secrets
    return text[:limit]

def main():
    print("BASE", BASE)
    client = make_client()
    status, resp, err = api_call(client, "POST", "/api/session/login", {
        "username": "admin",
        "password": PASSWORD,
    })
    print("LOGIN", status, "ok" if status == 200 else brief(resp))
    if status != 200:
        return

    for path in ("/api/plugins", "/api/plugins/builtins", "/api/plugins/loaded"):
        st, body, er = api_call(client, "GET", path)
        if path == "/api/plugins" and isinstance(body, dict):
            plugins = body.get("plugins") or []
            print(f"GET {path} {st} count={body.get('count')} listed={len(plugins)}")
            for p in plugins:
                cfg = p.get("config")
                cfg_keys = list(cfg.keys()) if isinstance(cfg, dict) else type(cfg).__name__
                status_obj = p.get("status") or {}
                print(
                    f"  name={p.get('name')} custom={p.get('isCustom')} enabled={p.get('enabled')} "
                    f"runtime={status_obj.get('status')} types={status_obj.get('types')} "
                    f"logs={len(status_obj.get('logs') or [])} cfg_keys={cfg_keys} "
                    f"path_set={bool(p.get('path'))}"
                )
                logs = status_obj.get("logs") or []
                if logs:
                    print("    log0:", str(logs[0])[:180])
        else:
            print(f"GET {path} {st} {brief(body)}")

    suffix = uuid.uuid4().hex[:8]
    name = f"livetest-{suffix}"

    cases = [
        ("empty name", {"name": "", "enabled": True, "path": "/tmp/x.so"}),
        ("bad placement", {"name": name + "-badplace", "enabled": False, "path": "/tmp/x.so", "placement": "middle"}),
        ("windows path disabled", {"name": name + "-win", "enabled": False, "path": r"D:\unifai_project\scratch\no-such-plugin.so"}),
        ("missing file enabled", {"name": name + "-missing", "enabled": True, "path": r"D:\unifai_project\scratch\no-such-plugin.so"}),
        ("http url disabled", {"name": name + "-url", "enabled": False, "path": "https://example.invalid/plugin.so"}),
    ]
    created = []
    for label, payload in cases:
        st, body, er = api_call(client, "POST", "/api/plugins", payload)
        msg = ""
        if isinstance(body, dict):
            msg = body.get("message") or body.get("error") or ""
            if body.get("plugin"):
                created.append(payload["name"])
        print(f"CREATE {label} -> {st} {msg or brief(body, 220)}")

    # duplicate
    if created:
        st, body, er = api_call(client, "POST", "/api/plugins", {
            "name": created[0], "enabled": False, "path": "/tmp/x.so",
        })
        msg = body.get("error") or body.get("message") if isinstance(body, dict) else brief(body)
        print(f"CREATE duplicate -> {st} {msg}")

    # update missing builtin-looking name that does not exist: governance should exist
    st, body, er = api_call(client, "GET", "/api/plugins/governance")
    if isinstance(body, dict):
        print(
            f"GET governance {st} enabled={body.get('enabled')} custom={body.get('isCustom')} "
            f"runtime={(body.get('status') or {}).get('status')}"
        )
    else:
        print("GET governance", st, brief(body))

    # toggle a throwaway disabled plugin on then off if we created one
    for n in created:
        st, body, er = api_call(client, "PUT", f"/api/plugins/{n}", {
            "enabled": True,
            "path": r"D:\unifai_project\scratch\no-such-plugin.so",
            "config": {},
        })
        msg = ""
        if isinstance(body, dict):
            msg = body.get("error") or body.get("message") or ""
        print(f"UPDATE enable missing file {n} -> {st} {msg or brief(body, 220)}")

        st, body, er = api_call(client, "PUT", f"/api/plugins/{n}", {
            "enabled": False,
            "config": {"probe": True},
        })
        msg = ""
        if isinstance(body, dict):
            msg = body.get("error") or body.get("message") or ""
            plug = body.get("plugin") or {}
            print(f"UPDATE disable {n} -> {st} enabled={plug.get('enabled')} msg={msg}")
        else:
            print(f"UPDATE disable {n} -> {st} {brief(body)}")

    # bad json is client-side; invalid config object
    if created:
        st, body, er = api_call(client, "PUT", f"/api/plugins/{created[0]}", {
            "enabled": False,
            "config": "not-an-object",
        })
        msg = body.get("error") or body.get("message") if isinstance(body, dict) else brief(body)
        print(f"UPDATE bad config -> {st} {msg}")

    for n in created:
        st, body, er = api_call(client, "DELETE", f"/api/plugins/{n}")
        msg = body.get("error") or body.get("message") if isinstance(body, dict) else brief(body)
        print(f"DELETE {n} -> {st} {msg}")

    st, body, er = api_call(client, "DELETE", f"/api/plugins/does-not-exist-{suffix}")
    msg = body.get("error") or body.get("message") if isinstance(body, dict) else brief(body)
    print(f"DELETE missing -> {st} {msg}")

    # confirm list still loads
    st, body, er = api_call(client, "GET", "/api/plugins")
    names = []
    if isinstance(body, dict):
        names = [p.get("name") for p in (body.get("plugins") or [])]
    leftover = [n for n in created if n in names]
    print("LEFTOVER", leftover, "total", len(names))

if __name__ == "__main__":
    main()
