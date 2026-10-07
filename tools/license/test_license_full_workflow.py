#!/usr/bin/env python3
"""
Comprehensive End-to-End Test Suite for Gateway Enterprise Guard License System.

Tests all scenarios requested:
1. Key Creation & Signing (2 seats, Ed25519)
2. Decryption & Tampering Attacks (seats altered, forged key, tampered signature)
3. Expiry Date Enforcement (today / past expiry rejection)
4. Guard Connection Limit (Strict 2-guard limit, 3rd blocked)
5. Guard ON/OFF (Pause / Resume) & Dynamic Seat Reallocation
6. License Upgrade (Upgrade from 2 seats to 5 seats, allowing up to 5 guards)
"""

import os
import sys
import json
import base64
import uuid
from datetime import datetime, timezone, timedelta
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives import serialization

# Add tools/license to sys.path
SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

from issue_license import issue_license
from verify_license import verify_license_file

class MockGuardFleetManager:
    """Simulates Gateway BrowserAIManager license & seat quota enforcement logic."""
    def __init__(self, master_pubkey_bytes: bytes):
        self.pubkey = ed25519.Ed25519PublicKey.from_public_bytes(master_pubkey_bytes)
        self.active_license = None
        self.agents = {}  # agent_id -> {"status": "active"|"paused"|"uninstalled", "hostname": str, "mac": str}

    def verify_envelope(self, env_dict: dict) -> dict:
        if "payload_b64" not in env_dict or "signature" not in env_dict:
            raise ValueError("Malformed license: missing payload_b64 or signature")
        
        raw_payload = base64.b64decode(env_dict["payload_b64"])
        raw_sig = base64.b64decode(env_dict["signature"])
        
        # Verify Ed25519 signature
        self.pubkey.verify(raw_sig, raw_payload)
        
        payload = json.loads(raw_payload.decode("utf-8"))
        
        # Verify Expiry
        if "expires_at" in payload:
            exp_str = payload["expires_at"]
            try:
                exp_dt = datetime.fromisoformat(exp_str.replace("Z", "+00:00"))
            except Exception:
                exp_dt = datetime.strptime(exp_str[:10], "%Y-%m-%d").replace(tzinfo=timezone.utc)
            
            if datetime.now(timezone.utc) > exp_dt:
                raise ValueError(f"LICENSE_EXPIRED: license expired on {exp_str}")
        
        return payload

    def activate_license(self, env_dict: dict):
        payload = self.verify_envelope(env_dict)
        self.active_license = payload
        return payload

    def get_active_seat_count(self) -> int:
        return sum(1 for a in self.agents.values() if a["status"] == "active")

    def register_or_heartbeat_agent(self, agent_id: str, hostname: str, mac: str) -> tuple[bool, str]:
        if not self.active_license:
            raise RuntimeError("LICENSE_INACTIVE: No license activated")
        
        max_seats = self.active_license["max_seats"]
        
        # If this agent is already active, no new seat consumed
        existing = self.agents.get(agent_id)
        if existing and existing["status"] == "active":
            return True, f"Agent {agent_id} heartbeat OK (already active)"
        
        # Check if same physical machine (mac + hostname) is already active
        for aid, ainfo in self.agents.items():
            if ainfo["status"] == "active" and ainfo["mac"].lower() == mac.lower() and ainfo["hostname"].lower() == hostname.lower():
                self.agents[agent_id] = {"status": "active", "hostname": hostname, "mac": mac}
                return True, f"Hardware match {hostname} heartbeat OK (already active)"
        
        # New agent or resuming agent -> check capacity
        active_count = self.get_active_seat_count()
        if active_count >= max_seats:
            return False, f"SEAT_LIMIT_REACHED: Enterprise on-premise license capacity reached ({active_count}/{max_seats} active laptops in use)."
        
        self.agents[agent_id] = {"status": "active", "hostname": hostname, "mac": mac}
        return True, f"Agent {agent_id} registered successfully ({active_count + 1}/{max_seats} seats used)"

    def pause_agent(self, agent_id: str) -> tuple[bool, str]:
        if agent_id not in self.agents:
            return False, "Agent not found"
        self.agents[agent_id]["status"] = "paused"
        return True, f"Agent {agent_id} paused (turned OFF) - seat freed up"

    def resume_agent(self, agent_id: str) -> tuple[bool, str]:
        if agent_id not in self.agents:
            return False, "Agent not found"
        
        max_seats = self.active_license["max_seats"]
        active_count = self.get_active_seat_count()
        if active_count >= max_seats:
            return False, f"SEAT_LIMIT_REACHED: Cannot resume {agent_id} ({active_count}/{max_seats} active laptops in use)."
        
        self.agents[agent_id]["status"] = "active"
        return True, f"Agent {agent_id} resumed (turned ON) ({active_count + 1}/{max_seats} seats used)"


def run_all_tests():
    print("=" * 70)
    print(" GATEWAY ENTERPRISE LICENSE & GUARD SEAT QUOTA - FULL TEST SUITE")
    print("=" * 70)
    
    # 1. Load Vendor Master Public Key
    pub_file = SCRIPT_DIR / "master_public.key"
    with open(pub_file, "rb") as f:
        pubkey_obj = serialization.load_pem_public_key(f.read())
    pub_raw_bytes = pubkey_obj.public_bytes(serialization.Encoding.Raw, serialization.PublicFormat.Raw)
    
    fleet = MockGuardFleetManager(pub_raw_bytes)
    passes = 0
    total = 0

    def assert_test(cond, title):
        nonlocal passes, total
        total += 1
        if cond:
            print(f" [PASS] Test {total:02d}: {title}")
            passes += 1
        else:
            print(f" [FAIL] Test {total:02d}: {title}")
            raise AssertionError(f"Test failed: {title}")

    # -------------------------------------------------------------
    # TEST PHASE 1: 2-Guard Key Creation & Valid Activation
    # -------------------------------------------------------------
    print("\n--- PHASE 1: Key Creation & Activation (2 Seats) ---")
    valid_expiry = (datetime.now(timezone.utc) + timedelta(days=90)).strftime("%Y-%m-%d")
    res2 = issue_license(
        client_name="Test Enterprise Corp",
        seats=2,
        expiry_date=valid_expiry,
        output_path=str(SCRIPT_DIR / "test_active_2_seat.lic")
    )
    env2 = res2["envelope"]
    assert_test(env2["payload"]["max_seats"] == 2, "Issued license has max_seats = 2")
    assert_test(len(env2["signature"]) > 40, "Cryptographic Ed25519 signature is generated")
    
    activated = fleet.activate_license(env2)
    assert_test(activated["max_seats"] == 2, "Activated license correctly recognized with 2 seats")

    # -------------------------------------------------------------
    # TEST PHASE 2: Tamper & Decrypt / Attack Verification
    # -------------------------------------------------------------
    print("\n--- PHASE 2: Decryption / Tampering / Forgery Attack Tests ---")
    
    # Attack 2.1: Attacker changes seats from 2 to 999 without private key
    tampered_env = json.loads(json.dumps(env2))
    tampered_payload = json.loads(base64.b64decode(tampered_env["payload_b64"]).decode())
    tampered_payload["max_seats"] = 999
    tampered_env["payload_b64"] = base64.b64encode(json.dumps(tampered_payload).encode()).decode()
    
    tamper_blocked = False
    try:
        fleet.verify_envelope(tampered_env)
    except Exception as e:
        tamper_blocked = True
    assert_test(tamper_blocked, "Tampering Attack: Changing seats 2 -> 999 rejected with signature invalid")

    # Attack 2.2: Attacker alters signature string
    bad_sig_env = json.loads(json.dumps(env2))
    bad_sig_env["signature"] = base64.b64encode(b"invalid_signature_bytes_1234567890123456789012345678901234567890").decode()
    bad_sig_blocked = False
    try:
        fleet.verify_envelope(bad_sig_env)
    except Exception:
        bad_sig_blocked = True
    assert_test(bad_sig_blocked, "Tampering Attack: Altered/forged signature rejected")

    # Attack 2.3: Attacker generates rogue keypair and signs
    rogue_priv = ed25519.Ed25519PrivateKey.generate()
    rogue_sig = rogue_priv.sign(base64.b64decode(env2["payload_b64"]))
    rogue_env = json.loads(json.dumps(env2))
    rogue_env["signature"] = base64.b64encode(rogue_sig).decode()
    rogue_blocked = False
    try:
        fleet.verify_envelope(rogue_env)
    except Exception:
        rogue_blocked = True
    assert_test(rogue_blocked, "Forgery Attack: Key signed with unauthorized private key rejected")

    # -------------------------------------------------------------
    # TEST PHASE 3: Expiry Date Enforcement (Today / Past timing)
    # -------------------------------------------------------------
    print("\n--- PHASE 3: Expiry Date Verification ---")
    past_expiry = (datetime.now(timezone.utc) - timedelta(hours=2)).strftime("%Y-%m-%dT%H:%M:%SZ")
    
    # Create an expired payload
    priv_file = SCRIPT_DIR / "master_private.key"
    with open(priv_file, "rb") as f:
        priv_key = serialization.load_pem_private_key(f.read(), password=None)
    
    exp_payload = {
        "version": "1.0",
        "license_id": "YP-EXPIRED-TEST",
        "client_name": "Expired Corp",
        "max_seats": 2,
        "issued_at": (datetime.now(timezone.utc) - timedelta(days=30)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "expires_at": past_expiry
    }
    exp_json = json.dumps(exp_payload, sort_keys=True, separators=(",", ":")).encode()
    exp_sig = base64.b64encode(priv_key.sign(exp_json)).decode()
    exp_env = {
        "format": "gateway_enterprise_license_v1",
        "payload": exp_payload,
        "payload_b64": base64.b64encode(exp_json).decode(),
        "signature": exp_sig
    }
    
    exp_blocked = False
    try:
        fleet.verify_envelope(exp_env)
    except Exception as e:
        if "LICENSE_EXPIRED" in str(e):
            exp_blocked = True
    assert_test(exp_blocked, "Expired Key: Key with expired timing is rejected with LICENSE_EXPIRED")

    # -------------------------------------------------------------
    # TEST PHASE 4: 2-System Guard Limit Enforcement
    # -------------------------------------------------------------
    print("\n--- PHASE 4: Guard Limit Enforcement (Strict 2 Systems) ---")
    
    # Connect Guard 1
    ok1, msg1 = fleet.register_or_heartbeat_agent("guard-01", "Sakthi-Laptop-01", "00:11:22:33:44:01")
    assert_test(ok1 and fleet.get_active_seat_count() == 1, "System 1 (Guard 1) connects -> ALLOWED (1/2 seats used)")

    # Connect Guard 2
    ok2, msg2 = fleet.register_or_heartbeat_agent("guard-02", "Sakthi-Laptop-02", "00:11:22:33:44:02")
    assert_test(ok2 and fleet.get_active_seat_count() == 2, "System 2 (Guard 2) connects -> ALLOWED (2/2 seats used - FULL)")

    # Connect Guard 3 -> MUST BE BLOCKED!
    ok3, msg3 = fleet.register_or_heartbeat_agent("guard-03", "Sakthi-Laptop-03", "00:11:22:33:44:03")
    assert_test(not ok3 and "SEAT_LIMIT_REACHED" in msg3, "System 3 (Guard 3) connects -> BLOCKED! (Strict 2-guard limit enforced)")
    assert_test(fleet.get_active_seat_count() == 2, "Active guards remain strictly 2")

    # Re-heartbeat from Guard 1 (existing guard should not be blocked)
    hb_ok, _ = fleet.register_or_heartbeat_agent("guard-01", "Sakthi-Laptop-01", "00:11:22:33:44:01")
    assert_test(hb_ok, "Existing Guard 1 heartbeat causes NO new seat consumption")

    # -------------------------------------------------------------
    # TEST PHASE 5: Guard ON/OFF (Pause/Resume) & Reassigning
    # -------------------------------------------------------------
    print("\n--- PHASE 5: Guard ON/OFF & Reassigning Tests ---")
    
    # Turn Guard 1 OFF (Pause)
    pok, pmsg = fleet.pause_agent("guard-01")
    assert_test(pok and fleet.get_active_seat_count() == 1, "Turn Guard 1 OFF (Paused) -> Seat freed up! (1/2 seats used)")

    # Now System 3 (Guard 3) connects -> SHOULD BE ALLOWED now!
    ok3_retry, msg3_retry = fleet.register_or_heartbeat_agent("guard-03", "Sakthi-Laptop-03", "00:11:22:33:44:03")
    assert_test(ok3_retry and fleet.get_active_seat_count() == 2, "Assign new System 3 -> ALLOWED! Takes freed slot (2/2 seats used)")

    # Now Guard 1 tries to turn back ON (Resume) -> MUST BE BLOCKED (capacity is full again)
    res_ok, res_msg = fleet.resume_agent("guard-01")
    assert_test(not res_ok and "SEAT_LIMIT_REACHED" in res_msg, "Guard 1 Resume while full -> BLOCKED! Cannot exceed limit")

    # Turn Guard 2 OFF
    fleet.pause_agent("guard-02")
    assert_test(fleet.get_active_seat_count() == 1, "Turn Guard 2 OFF -> Seat freed up (1/2 seats used)")

    # Now Guard 1 resumes -> ALLOWED!
    res_ok2, _ = fleet.resume_agent("guard-01")
    assert_test(res_ok2 and fleet.get_active_seat_count() == 2, "Guard 1 Resume after slot freed -> ALLOWED! (2/2 seats used)")

    # -------------------------------------------------------------
    # TEST PHASE 6: Change License Key from 2 to 5 Guards
    # -------------------------------------------------------------
    print("\n--- PHASE 6: License Key Upgrade (2 -> 5 Seats) ---")
    
    res5 = issue_license(
        client_name="Test Enterprise Corp",
        seats=5,
        expiry_date=valid_expiry,
        output_path=str(SCRIPT_DIR / "test_active_5_seat.lic")
    )
    env5 = res5["envelope"]
    activated5 = fleet.activate_license(env5)
    assert_test(activated5["max_seats"] == 5, "Upgraded license activated: max_seats = 5")

    # Resume Guard 2
    g2_res, _ = fleet.resume_agent("guard-02")
    assert_test(g2_res and fleet.get_active_seat_count() == 3, "Resume Guard 2 with 5-seat key -> ALLOWED (3/5 seats used)")

    # Connect Guard 4
    ok4, _ = fleet.register_or_heartbeat_agent("guard-04", "Sakthi-Laptop-04", "00:11:22:33:44:04")
    assert_test(ok4 and fleet.get_active_seat_count() == 4, "System 4 (Guard 4) connects -> ALLOWED (4/5 seats used)")

    # Connect Guard 5
    ok5, _ = fleet.register_or_heartbeat_agent("guard-05", "Sakthi-Laptop-05", "00:11:22:33:44:05")
    assert_test(ok5 and fleet.get_active_seat_count() == 5, "System 5 (Guard 5) connects -> ALLOWED (5/5 seats used - FULL)")

    # Connect Guard 6 -> MUST BE BLOCKED!
    ok6, msg6 = fleet.register_or_heartbeat_agent("guard-06", "Sakthi-Laptop-06", "00:11:22:33:44:06")
    assert_test(not ok6 and "SEAT_LIMIT_REACHED" in msg6, "System 6 (Guard 6) connects -> BLOCKED! (5-seat limit enforced)")

    # Final summary
    print("\n" + "=" * 70)
    print(f" ALL TESTS COMPLETED: {passes} / {total} PASSED (100% SUCCESS)")
    print("=" * 70)

    # Cleanup temporary test .lic files
    for p in [SCRIPT_DIR / "test_active_2_seat.lic", SCRIPT_DIR / "test_active_5_seat.lic"]:
        if p.exists():
            p.unlink()

if __name__ == "__main__":
    run_all_tests()
