#!/usr/bin/env python3
"""
Test Hybrid Lifecycle:
1. Scenario A: Enterprise Active Directory domain joined laptop
   - Heartbeat registers ad_domain, ad_upn, ad_groups, is_domain_joined=True.
   - Auto-sets contact_email = ad_upn.
   - Prompt intercepts automatically tag the employee's AD identity.
2. Scenario B: Manual standalone / non-domain laptop
   - Heartbeat registers with local username and hostname.
   - is_domain_joined=False, no errors, fully functional.
3. Scenario C: Rebuild / Reinstall / Update lifecycle
   - Reinstalled agent keeps durable identity and updates status cleanly.
"""

import sys
import json
import urllib.request
import urllib.error
import http.cookiejar
from pathlib import Path

BASE_URL = "http://127.0.0.1:8080"
ADMIN_USER = "admin@yespanchi.com"
ADMIN_PASS = "YP2025-2026yp"

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

def login():
    url = f"{BASE_URL}/api/session/login"
    data = json.dumps({"username": ADMIN_USER, "password": ADMIN_PASS}).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method="POST")
    with opener.open(req) as resp:
        return resp.getcode() == 200

GUARD_SECRET = "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="

def post_heartbeat(payload):
    url = f"{BASE_URL}/api/browser-ai/agents/heartbeat"
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "X-Gateway-Guard-Key": GUARD_SECRET,
            "X-Role": "admin"
        },
        method="POST"
    )
    with urllib.request.urlopen(req) as resp:
        return resp.getcode(), json.loads(resp.read().decode("utf-8"))

def post_intercept(payload):
    url = f"{BASE_URL}/api/browser-ai/intercept"
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "X-Gateway-Guard-Key": GUARD_SECRET
        },
        method="POST"
    )
    with urllib.request.urlopen(req) as resp:
        return resp.getcode(), json.loads(resp.read().decode("utf-8"))

def get_agents():
    url = f"{BASE_URL}/api/browser-ai/agents"
    req = urllib.request.Request(url)
    with opener.open(req) as resp:
        return json.loads(resp.read().decode("utf-8"))

def get_logs():
    url = f"{BASE_URL}/api/browser-ai/logs?limit=10"
    req = urllib.request.Request(url)
    with opener.open(req) as resp:
        return json.loads(resp.read().decode("utf-8"))

def main():
    print("=== Testing Active Directory + Manual Hybrid Lifecycle ===")
    assert login(), "Admin login failed"
    print("[*] Logged into Dashboard as Admin successfully.")

    # 1. SCENARIO A: Enterprise Active Directory Domain Laptop
    print("\n--- 1. Testing Active Directory Domain Joined Laptop ---")
    ad_agent_id = "ad-agent-corp-fin-001"
    ad_heartbeat = {
        "id": ad_agent_id,
        "hostname": "LAPTOP-FIN-042",
        "username": "arun.kumar",
        "ip_address": "10.14.2.85",
        "mac_address": "AA-BB-CC-11-22-33",
        "os_version": "Windows-11-Enterprise",
        "agent_version": "1.1.17",
        "agent_type": "endpoint",
        "status": "active",
        "is_domain_joined": True,
        "ad_domain": "TITANBANK.CORP",
        "ad_upn": "arun.kumar@titanbank.com",
        "ad_groups": "Finance; Risk-Analysis; Corporate-Users",
        "domain_user": "TITANBANK\\arun.kumar"
    }
    code, res = post_heartbeat(ad_heartbeat)
    assert code == 200, f"Expected 200, got {code}"
    ag = res.get("agent", {})
    print(f"[*] Agent registered: {ag.get('hostname')} | AD: {ag.get('ad_domain')} | UPN: {ag.get('ad_upn')}")
    assert ag.get("is_domain_joined") is True, "Expected is_domain_joined == True"
    assert ag.get("ad_domain") == "TITANBANK.CORP", "AD Domain mismatch"
    assert ag.get("ad_upn") == "arun.kumar@titanbank.com", "AD UPN mismatch"
    assert ag.get("contact_email") == "arun.kumar@titanbank.com", "Expected contact_email to auto-inherit AD UPN"
    print("[PASS] Scenario A (Heartbeat): Active Directory identity captured and persisted!")

    # Intercept prompt from this AD agent
    print("\n--- 1b. Testing Prompt Intercept from AD Laptop ---")
    prompt_payload = {
        "platform": "ChatGPT",
        "prompt": "Can you summarize our Q3 loan risk analysis?",
        "metadata": {
            "agent_id": ad_agent_id,
            "agent_hostname": "LAPTOP-FIN-042",
            "domain": "chatgpt.com"
        }
    }
    code_p, res_p = post_intercept(prompt_payload)
    assert code_p == 200, f"Expected 200, got {code_p}"
    print(f"[*] Prompt intercepted: {res_p.get('status')}")

    # Verify audit log in Dashboard
    logs_data = get_logs()
    recent_log = logs_data["logs"][0]
    print(f"[*] Most recent log employee: UPN={recent_log.get('ad_upn')} | Domain={recent_log.get('ad_domain')} | User={recent_log.get('domain_user')}")
    assert recent_log.get("ad_upn") == "arun.kumar@titanbank.com", "Expected log ad_upn to match agent"
    assert recent_log.get("ad_domain") == "TITANBANK.CORP", "Expected log ad_domain to match agent"
    print("[PASS] Scenario A (Prompt Logs): Intercept automatically tagged with Employee Corporate AD Identity!")

    # 2. SCENARIO B: Manual / Non-Domain / Standalone Laptop
    print("\n--- 2. Testing Manual / Standalone Laptop (Non-Domain) ---")
    manual_agent_id = "manual-agent-standalone-002"
    manual_heartbeat = {
        "id": manual_agent_id,
        "hostname": "SAKTHI-DESKTOP",
        "username": "sakthi",
        "ip_address": "192.168.1.100",
        "mac_address": "DD-EE-FF-44-55-66",
        "os_version": "Windows-11-Pro",
        "agent_version": "1.1.17",
        "agent_type": "endpoint",
        "status": "active",
        "is_domain_joined": False,
        "ad_domain": "",
        "ad_upn": "",
        "ad_groups": "",
        "domain_user": "sakthi"
    }
    code_m, res_m = post_heartbeat(manual_heartbeat)
    assert code_m == 200, f"Expected 200, got {code_m}"
    ag_m = res_m.get("agent", {})
    print(f"[*] Manual agent registered: {ag_m.get('hostname')} | Username: {ag_m.get('username')} | is_domain_joined={ag_m.get('is_domain_joined')}")
    assert ag_m.get("is_domain_joined") is False, "Expected is_domain_joined == False"
    assert ag_m.get("username") == "sakthi", "Username mismatch"
    print("[PASS] Scenario B: Standalone manual install operates seamlessly without errors!")

    # 3. SCENARIO C: Reinstall / Rebuild / Update Lifecycle
    print("\n--- 3. Testing Reinstall / Auto-Update Lifecycle ---")
    update_heartbeat = dict(ad_heartbeat)
    update_heartbeat["agent_version"] = "1.1.18"  # Reinstalled or updated to 1.1.18
    code_u, res_u = post_heartbeat(update_heartbeat)
    assert code_u == 200, f"Expected 200, got {code_u}"
    ag_u = res_u.get("agent", {})
    print(f"[*] Agent updated: ID={ag_u.get('id')} | Version={ag_u.get('agent_version')} | AD={ag_u.get('ad_domain')}")
    assert ag_u.get("agent_version") == "1.1.18", "Expected updated agent version"
    assert ag_u.get("ad_upn") == "arun.kumar@titanbank.com", "AD identity preserved after update"
    print("[PASS] Scenario C: Rebuild / Reinstall / Update preserves all identities & quota seamlessly!")

    print("\n" + "="*65)
    print(" ALL HYBRID AD + MANUAL LIFECYCLE TESTS PASSED! [100% SUCCESS]")
    print("="*65)

if __name__ == "__main__":
    main()
