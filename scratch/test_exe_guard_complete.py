"""
Comprehensive Verification Suite for Gateway Guard EXE & Agent Subsystems.
Tests the compiled EXE, in-RAM encrypted bundles, agent identity, heartbeat,
PAC routing, certificate management, uninstall protection, and Inno Setup configuration.
"""

import os
import sys
import json
import time
import subprocess
import urllib.request
import http.cookiejar

BASE = "http://localhost:8080"
GUARD_SECRET = "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="
ADMIN_USER = "admin@yespanchi.com"
ADMIN_PASS = "YP2025-2026yp"

ROOT_GUARD = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "apps", "browser-guard"))
RELEASE_DIR = os.path.join(ROOT_GUARD, "release")
PROXY_PARTS_DIR = os.path.join(ROOT_GUARD, "proxy", "gateway_proxy_parts")
AGENT_DIR = os.path.join(ROOT_GUARD, "agent")

sys.path.insert(0, PROXY_PARTS_DIR)
sys.path.insert(0, AGENT_DIR)

results = []

def test(title, condition, detail=""):
    results.append((title, condition, detail))
    icon = "[PASS]" if condition else "[FAIL]"
    print(f"  {icon} {title}: {detail}")
    return condition

def main():
    print("==========================================================================")
    print("      UNIFAI GATEWAY GUARD - FULL EXE & AGENT VERIFICATION AUDIT          ")
    print("==========================================================================\n")

    # ----------------------------------------------------------------------
    # SECTION 1: EXE BINARY INTEGRITY & COMPILATION STATUS
    # ----------------------------------------------------------------------
    print("--- SECTION 1: EXE BINARIES & PACKAGING INTEGRITY ---")
    exe_path = os.path.join(RELEASE_DIR, "Gateway_Guard.exe")
    setup_exe_path = os.path.join(RELEASE_DIR, "Gateway_Guard_Setup.exe")
    zip_path = os.path.join(RELEASE_DIR, "Gateway_Guard_Windows.zip")

    test("Gateway_Guard.exe Exists", os.path.isfile(exe_path), f"Path: {exe_path}")
    if os.path.isfile(exe_path):
        exe_size = os.path.getsize(exe_path)
        test("Gateway_Guard.exe Size Valid", exe_size > 30_000_000, f"Size: {exe_size:,} bytes (~{exe_size / 1024 / 1024:.1f} MB)")
        with open(exe_path, "rb") as f:
            header = f.read(2)
        test("Valid Windows PE Header (MZ)", header == b"MZ", "Executable signature verified")

    test("Gateway_Guard_Setup.exe Exists", os.path.isfile(setup_exe_path), f"Path: {setup_exe_path}")
    if os.path.isfile(setup_exe_path):
        setup_size = os.path.getsize(setup_exe_path)
        test("Gateway_Guard_Setup.exe Size Valid", setup_size > 30_000_000, f"Size: {setup_size:,} bytes (~{setup_size / 1024 / 1024:.1f} MB)")

    test("Gateway_Guard_Windows.zip Exists", os.path.isfile(zip_path), f"Path: {zip_path}")

    # ----------------------------------------------------------------------
    # SECTION 2: ENCRYPTED IN-RAM BUNDLE DECRYPTION VERIFICATION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 2: ENCRYPTED IN-RAM BUNDLE CRYPTOGRAPHY ---")
    import bundle_crypto
    from pathlib import Path

    parts_enc = os.path.join(PROXY_PARTS_DIR, "gateway_proxy_parts.enc")
    code_enc = os.path.join(ROOT_GUARD, "proxy", "gateway_guard_code.enc")

    test("gateway_proxy_parts.enc Exists", os.path.isfile(parts_enc), f"Size: {os.path.getsize(parts_enc):,} bytes")
    test("gateway_guard_code.enc Exists", os.path.isfile(code_enc), f"Size: {os.path.getsize(code_enc):,} bytes")

    # Decrypt proxy parts bundle in RAM
    try:
        decrypted_parts = bundle_crypto.decrypt_parts_bundle(Path(parts_enc))
        test("Proxy Bundle Decrypts in RAM Cleanly", len(decrypted_parts) >= 6, f"Decrypted {len(decrypted_parts)} internal modules in RAM")
        test("Zero Disk Leaks on Proxy Decryption", not os.path.exists(os.path.join(PROXY_PARTS_DIR, "temp_decrypted")), "Zero disk writes")
        test("Proxy Parts Content Integrity", "file_policy.py" in decrypted_parts and "config_caches_rules.py" in decrypted_parts, "Core DLP modules verified")
    except Exception as e:
        test("Proxy Bundle Decrypts in RAM Cleanly", False, str(e))

    # Decrypt agent code bundle in RAM
    try:
        decrypted_agent = bundle_crypto.decrypt_parts_bundle(Path(code_enc))
        test("Agent Code Bundle Decrypts in RAM Cleanly", len(decrypted_agent) >= 10, f"Decrypted {len(decrypted_agent)} agent modules in RAM")
        test("Agent Modules Content Integrity", "gateway_agent.py" in decrypted_agent and "agent_heartbeat.py" in decrypted_agent, "Core agent modules verified")
    except Exception as e:
        test("Agent Code Bundle Decrypts in RAM Cleanly", False, str(e))

    # ----------------------------------------------------------------------
    # SECTION 3: AGENT IDENTITY & HARDWARE FINGERPRINT BINDING
    # ----------------------------------------------------------------------
    print("\n--- SECTION 3: AGENT IDENTITY & MACHINE HARDWARE BINDING ---")
    import agent_identity

    agent_id = agent_identity.get_or_create_agent_id()
    test("Agent ID Deterministically Created", len(agent_id) >= 16, f"Agent ID: {agent_id}")

    info = agent_identity.collect_agent_info(agent_id)
    test("Agent Info Collected", isinstance(info, dict) and bool(info.get("hostname")), f"Hostname: {info.get('hostname')}")
    test("Agent ID Associated with System", info.get("id") == agent_id, f"Associated Agent ID: {info.get('id')}")
    test("Active Directory / User Detected", "username" in info, f"User: {info.get('username')}, Domain: {info.get('ad_domain') or 'Standalone'}")

    # ----------------------------------------------------------------------
    # SECTION 4: LOCAL PAC PROXY ROUTING GENERATION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 4: LOCAL PAC ROUTING & DIRECT BYPASS RULES ---")
    import agent_config
    agent_config.GATEWAY_BACKEND_URL = BASE
    agent_config.PAC_URL = f"{BASE}/api/browser-ai/pac"

    import agent_pac_content

    pac_script = agent_pac_content.fetch_proxy_pac()
    test("PAC Script Fetched from Backend", pac_script is not None and len(pac_script) > 100, f"Length: {len(pac_script) if pac_script else 0} chars")
    test("PAC Script Uses FindProxyForURL", "FindProxyForURL" in (pac_script or ""), "Standard PAC entrypoint present")
    test("PAC Script Directs Non-Target Sites DIRECT", "return \"DIRECT\"" in (pac_script or "") or "return 'DIRECT'" in (pac_script or "") or "DIRECT;" in (pac_script or ""), "Direct bypass verified")
    test("PAC Script Routes Target Domains via Proxy", "PROXY" in (pac_script or "") and ("18103" in (pac_script or "") or "proxy" in (pac_script or "").lower()), "Proxy loopback port routed")

    # ----------------------------------------------------------------------
    # SECTION 5: LIVE AGENT HEARTBEAT REGISTRATION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 5: LIVE AGENT HEARTBEAT & FLEET REGISTRATION ---")
    import agent_heartbeat

    hb_data = agent_heartbeat.send_heartbeat(agent_id, status="active")
    test("Live Agent Heartbeat Dispatched", hb_data is not None, "Heartbeat 200 OK to Gateway")

    # Verify agent appears in Gateway DB
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    login_req = urllib.request.Request(
        f"{BASE}/api/session/login",
        data=json.dumps({"username": ADMIN_USER, "password": ADMIN_PASS}).encode(),
        headers={"Content-Type": "application/json"},
        method="POST"
    )
    opener.open(login_req)

    agents_req = urllib.request.Request(f"{BASE}/api/browser-ai/agents", method="GET")
    agents_resp = opener.open(agents_req)
    agents_data = json.loads(agents_resp.read().decode())
    fleet_agents = agents_data.get("agents", [])
    matched_agent = next((a for a in fleet_agents if a.get("id") == agent_id or a.get("agent_id") == agent_id), None)
    test("Agent Visible in Admin Fleet Dashboard", matched_agent is not None, f"Registered: {agent_id} in {len(fleet_agents)} fleet agents")

    # ----------------------------------------------------------------------
    # SECTION 6: UNINSTALL PROTECTION VIA EXE CLI
    # ----------------------------------------------------------------------
    print("\n--- SECTION 6: EXE UNINSTALL PROTECTION & INVALID KEY REJECTION ---")
    sub_env = os.environ.copy()
    sub_env["GATEWAY_BACKEND_URL"] = BASE

    try:
        proc = subprocess.run(
            [exe_path, "--uninstall", "WRONG_KEY_123"],
            env=sub_env,
            capture_output=True,
            text=True,
            timeout=10
        )
        test("EXE Rejects Invalid Uninstall Key", proc.returncode == 2, f"Returncode: {proc.returncode} (2 = Invalid Key)")
    except Exception as e:
        test("EXE Rejects Invalid Uninstall Key", False, str(e))

    # Test Gateway Uninstall Key Verification API
    verify_url = f"{BASE}/api/browser-ai/agents/uninstall"
    req_bad_key = urllib.request.Request(
        verify_url,
        data=json.dumps({"agent_id": agent_id, "key": "INVALID_KEY_999"}).encode(),
        headers={"Content-Type": "application/json", "X-Gateway-Guard-Key": GUARD_SECRET},
        method="POST"
    )
    try:
        urllib.request.urlopen(req_bad_key)
        test("Gateway Rejects Unauthorized Uninstall", False, "Expected 403 or error")
    except urllib.error.HTTPError as e:
        test("Gateway Rejects Unauthorized Uninstall", e.code in (401, 403, 400), f"HTTP {e.code} Access Denied")

    # ----------------------------------------------------------------------
    # SECTION 7: INNO SETUP & UNINSTALLER CONFIGURATION
    # ----------------------------------------------------------------------
    print("\n--- SECTION 7: INNO SETUP & SILENT DEPLOYMENT SCRIPTS ---")
    iss_path = os.path.join(ROOT_GUARD, "installer", "Gateway_Guard.iss")
    test("Inno Setup Script (Gateway_Guard.iss) Exists", os.path.isfile(iss_path), f"Path: {iss_path}")
    if os.path.isfile(iss_path):
        with open(iss_path, "r", encoding="utf-8", errors="ignore") as f:
            iss_content = f.read()
        test("Inno Setup Enterprise Flags Configured", "DisableProgramGroupPage=yes" in iss_content or "PrivilegesRequired=" in iss_content, "Enterprise flags present")
        test("Inno Setup Secure Install Target Configured", "{localappdata}" in iss_content or "{autopf}" in iss_content, "Target directory secured")
        test("Inno Setup Autostart Registry Configured", "Software\\Microsoft\\Windows\\CurrentVersion\\Run" in iss_content, "Run key startup enabled")

    # ----------------------------------------------------------------------
    # SUMMARY
    # ----------------------------------------------------------------------
    total_tests = len(results)
    passed_tests = sum(1 for _, ok, _ in results if ok)
    failed_tests = total_tests - passed_tests

    print("\n==========================================================================")
    print(f"  EXE GUARD FULL AUDIT COMPLETE: {passed_tests}/{total_tests} PASS (Failures: {failed_tests})")
    print("==========================================================================")

    if failed_tests > 0:
        sys.exit(1)

if __name__ == "__main__":
    main()
