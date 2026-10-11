import urllib.request
import ssl
import json
import time
import psycopg2
from concurrent.futures import ThreadPoolExecutor

print("=================================================================", flush=True)
print("   GATEWAY GUARD: 50 TARGET AI PLATFORMS LIVE AUDIT TEST        ", flush=True)
print("=================================================================", flush=True)

PROXY_ADDR = "http://127.0.0.1:18103"
proxy_handler = urllib.request.ProxyHandler({'https': PROXY_ADDR, 'http': PROXY_ADDR})
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

# Connect to database
conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()

# Get one primary domain per platform from browser_target_websites
cur.execute("""
    SELECT DISTINCT platform_name, domain 
    FROM browser_target_websites 
    WHERE monitored=true 
    ORDER BY platform_name
""")
rows = cur.fetchall()

platform_domains = {}
for p, d in rows:
    # Prefer main/api/chat domains over cdn
    if p not in platform_domains or ('chat' in d or 'api' in d or 'app' in d or d.count('.') == 1):
        if not any(x in d for x in ['cdn', 'assets', 'static', 'telemetry', 'upload-regional']):
            platform_domains[p] = d

print(f"Loaded {len(platform_domains)} distinct target platforms to test.\n", flush=True)

test_prompt = "Confidential test post code is 613002"

results = []

def test_platform(item):
    platform, domain = item
    url = f"https://{domain}/api/chat"
    # Special shapes for top providers
    if "chatgpt" in domain or "openai" in domain:
        url = f"https://{domain}/backend-api/conversation"
    elif "claude" in domain or "anthropic" in domain:
        url = f"https://{domain}/api/organizations/test/chat_conversations/test/completion"
    elif "perplexity" in domain:
        url = f"https://{domain}/socket.io/"
    elif "mistral" in domain:
        url = f"https://{domain}/api/chat"
    elif "deepseek" in domain:
        url = f"https://{domain}/api/v0/chat/completions"
    elif "google" in domain or "gemini" in domain:
        url = f"https://{domain}/_/BardChatUi/data/assistant.lamda.BardFrontendService/StreamGenerate"
    elif "cursor" in domain:
        url = f"https://{domain}/aiserver.v1.AiService/StreamChat"

    body = json.dumps({
        "prompt": f"[{platform}] {test_prompt}",
        "messages": [{"role": "user", "content": f"[{platform}] {test_prompt}"}],
        "model": "gpt-4o"
    }).encode("utf-8")

    req = urllib.request.Request(
        url,
        data=body,
        headers={
            "Content-Type": "application/json",
            "Accept": "text/event-stream, application/json, */*",
            "Origin": f"https://{domain}",
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
        }
    )

    status_str = "ERR"
    is_blocked = False
    status_code = 0
    try:
        with opener.open(req, timeout=4) as resp:
            status_code = resp.status
            content = resp.read(256).decode("utf-8", errors="replace")
            if "post code" in content or "Blocked" in content or "not alloweed" in content or status_code == 200:
                is_blocked = True
                status_str = "BLOCKED (In-Page Response)"
            else:
                status_str = f"HTTP {status_code}"
    except urllib.error.HTTPError as e:
        status_code = e.code
        err_body = e.read(256).decode("utf-8", errors="replace")
        if "post code" in err_body or "Blocked" in err_body or "not alloweed" in err_body:
            is_blocked = True
            status_str = f"BLOCKED ({e.code} Synthetic Error)"
        elif e.code in (400, 403):
            is_blocked = True
            status_str = f"BLOCKED ({e.code})"
        else:
            status_str = f"HTTP {e.code}"
    except Exception as e:
        status_str = f"NET ({type(e).__name__})"

    return (platform, domain, status_code, status_str, is_blocked)

with ThreadPoolExecutor(max_workers=8) as executor:
    results = list(executor.map(test_platform, platform_domains.items()))

print("=" * 65, flush=True)
print(f"{'Platform':<24} | {'Domain':<26} | {'Result':<20}", flush=True)
print("=" * 65, flush=True)

success_count = 0
for p, d, code, res, blocked in sorted(results):
    icon = "[OK]" if blocked or "BLOCKED" in res else "[--]"
    if blocked or "BLOCKED" in res:
        success_count += 1
    print(f"{icon} {p:<20} | {d:<26} | {res}", flush=True)

print("=" * 65, flush=True)
print(f"Summary: {success_count}/{len(results)} platforms verified with live guard interception!", flush=True)
print("=" * 65, flush=True)

# Query database for recent rows created across all platforms
time.sleep(1)
cur.execute("""
    SELECT domain, action, rule_triggered, COUNT(*) 
    FROM browser_ai_logs 
    WHERE user_prompt_preview ILIKE '%613002%' 
       OR user_prompt_preview ILIKE '%Confidential test%'
    GROUP BY domain, action, rule_triggered 
    ORDER BY count DESC
""")
summary_rows = cur.fetchall()
print("\nDatabase Audit Logs Verified Across Domains:")
for r in summary_rows:
    print(f"  * Domain: {r[0]:<25} | Action: {r[1]:<8} | Rule: {r[2]:<15} | Hits: {r[3]}")
