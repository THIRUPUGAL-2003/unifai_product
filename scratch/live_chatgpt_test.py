import http.client
import json
import os
import ssl
import sys
import time
import urllib.request

PROXY_HOST = "127.0.0.1"
PROXY_PORT = 18103
CA_CERT = os.path.expanduser("~/.mitmproxy/mitmproxy-ca-cert.pem")

# Build a small valid PDF
PDF_BYTES = (
    b"%PDF-1.4\n"
    b"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
    b"2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj\n"
    b"3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Resources<<>>>>endobj\n"
    b"xref\n0 4\n0000000000 65535 f \n0000000009 00000 n \n0000000052 00000 n \n0000000101 00000 n \n"
    b"trailer<</Size 4/Root 1 0 R>>\nstartxref\n178\n%%EOF\n"
)

def send_via_proxy(host, path, data, headers, method="POST"):
    ctx = ssl.create_default_context(cafile=CA_CERT)
    conn = http.client.HTTPSConnection(PROXY_HOST, PROXY_PORT, context=ctx)
    conn.set_tunnel(host, 443)
    body = data if isinstance(data, (bytes, bytearray)) else (data.encode("utf-8") if data else b"")
    hdrs = dict(headers)
    hdrs["Host"] = host
    hdrs["Content-Length"] = str(len(body))
    try:
        conn.request(method, path, body=body, headers=hdrs)
        resp = conn.getresponse()
        resp_data = resp.read()
        conn.close()
        return resp.status, resp_data
    except Exception as e:
        conn.close()
        return 0, str(e).encode()

def main():
    print("=== LIVE PROXY VERIFICATION TEST ===")
    fid = "file-test-" + str(int(time.time()))
    prompt_text = "live verification test " + str(int(time.time()))[-4:]
    print(f"1. Target Conversation: https://chatgpt.com/c/6acb3579-f95c-83e8-aaf2-c76284e8fc43")
    print(f"2. File Name: 4658668226.pdf")
    print(f"3. Prompt: {prompt_text}")

    # Step 1: ChatGPT create-file handshake
    print("\n--> Sending ChatGPT file handshake...")
    hs_body = json.dumps({"file_name": "4658668226.pdf", "file_size": len(PDF_BYTES), "use_case": "my_files"})
    st1, r1 = send_via_proxy("chatgpt.com", "/backend-api/files", hs_body, {"Content-Type": "application/json", "Origin": "https://chatgpt.com"})
    print(f"Handshake status: {st1}")

    # Step 2: ChatGPT CDN PUT
    print("--> Sending ChatGPT CDN upload (raw PDF bytes)...")
    st2, r2 = send_via_proxy("files.oaiusercontent.com", f"/{fid}?se=2026&sp=cw&sig=abc", PDF_BYTES, {
        "Content-Type": "application/pdf",
        "Origin": "https://chatgpt.com",
        "Referer": "https://chatgpt.com/c/6acb3579-f95c-83e8-aaf2-c76284e8fc43",
        "x-ms-blob-type": "BlockBlob"
    }, method="PUT")
    print(f"CDN upload status: {st2}")

    # Step 3: ChatGPT background telemetry (which previously falsely captured as a file)
    print("--> Sending ChatGPT telemetry / lat / turn_analytics...")
    telem_body = json.dumps({"turn_analytics": {"event": "file_uploaded", "duration": 120}})
    st3, r3 = send_via_proxy("chatgpt.com", "/backend-api/lat/r", telem_body, {"Content-Type": "application/json", "Origin": "https://chatgpt.com"})
    print(f"Telemetry status: {st3}")

    # Step 4: ChatGPT Send Conversation with attachment
    print(f"--> Sending chat prompt '{prompt_text}' with attached file...")
    chat_body = json.dumps({
        "action": "next",
        "conversation_id": "6acb3579-f95c-83e8-aaf2-c76284e8fc43",
        "messages": [{
            "id": "msg-" + str(int(time.time())),
            "author": {"role": "user"},
            "content": {"content_type": "multimodal_text", "parts": [prompt_text]},
            "attachments": [{"id": fid, "name": "4658668226.pdf"}]
        }]
    })
    st4, r4 = send_via_proxy("chatgpt.com", "/backend-api/f/conversation", chat_body, {
        "Content-Type": "application/json",
        "Origin": "https://chatgpt.com",
        "Referer": "https://chatgpt.com/c/6acb3579-f95c-83e8-aaf2-c76284e8fc43"
    })
    print(f"Send status: {st4}")

    # Step 5: Wait 1 second and check backend logs
    time.sleep(2)
    from db_config import get_base_url, _CONFIG
    base_url = _CONFIG.get("SERVER_DOMAIN") or get_base_url()
    admin_user = _CONFIG.get("ADMIN_EMAIL", "")
    admin_pass = _CONFIG.get("ADMIN_PASSWORD", "")

    print(f"\n--> Authenticating to backend ({base_url}) to fetch Prompt Logs...")
    login_req = urllib.request.Request(
        f"{base_url}/api/session/login",
        data=json.dumps({"username": admin_user, "password": admin_pass}).encode(),
        headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(login_req, timeout=10) as l_resp:
        cookie = l_resp.headers.get("Set-Cookie", "").split(";")[0]

    print(f"--> Fetching Prompt Logs from {base_url}/api/browser-ai/logs?limit=8 ...")
    req = urllib.request.Request(f"{base_url}/api/browser-ai/logs?limit=8", headers={"Cookie": cookie})
    with urllib.request.urlopen(req, timeout=10) as resp:
        logs_data = json.loads(resp.read().decode())
    
    entries = logs_data.get("data", [])
    print(f"\n--- LATEST PROMPT LOGS ---")
    for i, e in enumerate(entries[:5]):
        print(f"[{i+1}] Platform: {e.get('platform')} | Prompt: {e.get('prompt')} | Action: {e.get('action')}")

    print("\n=== VERIFICATION RESULT ===")
    matches = [e for e in entries if prompt_text in (e.get('prompt') or "")]
    print(f"Found {len(matches)} log entries matching prompt '{prompt_text}':")
    for m in matches:
        print(f"  -> '{m.get('prompt')}'")
    
    if len(matches) == 1:
        p = matches[0].get('prompt') or ""
        if "4658668226.pdf" in p and "(1/" not in p and "(2/" not in p:
            print("\n>>> SUCCESS! Exactly 1 clean log row generated with filename 4658668226.pdf! No duplicates, no telemetry phantom files! <<<")
        else:
            print(f"Check format: {p}")
    else:
        print(f"Expected exactly 1 match, got {len(matches)}")

if __name__ == "__main__":
    main()
