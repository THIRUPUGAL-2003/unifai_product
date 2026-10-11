import psycopg2
import json
import base64
from datetime import datetime, timezone
from pathlib import Path

# Load master private key and issue deterministic license
from sys import path
path.insert(0, r'd:\unifai_project\tools\license')
from issue_license import issue_license

res = issue_license(
    client_name="Titan Secure Bank",
    seats=500,
    expiry_date="2028-12-31",
    server_id="SRV-D1FB08F6-AC2D7875",
    install_id="DB-CD1B2EC1-A1A5-4E38-BAD2-9F4471019380",
    license_id="YP-202610-36192996",
    revision=5,
    output_path=r"d:\unifai_project\gateway_license.lic",
)

payload = res["envelope"]["payload"]
token = res["envelope"]["license_key"]
sig = res["envelope"]["signature"]

print("Issued License Payload:", payload)

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()

# 1. Update browser_ai_license
features_json = json.dumps(payload["features"])
issued_at = payload["issued_at"]
expires_at = payload["expires_at"]
now = datetime.now(timezone.utc)

cur.execute("""
    UPDATE browser_ai_license
    SET license_id = %s,
        issuer = %s,
        product = %s,
        client_name = %s,
        tier = %s,
        max_seats = %s,
        server_hardware_id = %s,
        features = %s,
        raw_envelope = %s,
        signature = %s,
        issued_at = %s,
        expires_at = %s,
        updated_at = %s,
        updated_by = %s
    WHERE id = 'browser-ai-license-active';
""", (
    payload["license_id"],
    payload["issuer"],
    payload["product"],
    payload["client_name"],
    payload["tier"],
    payload["max_seats"],
    payload["server_hardware_id"],
    features_json,
    token,
    sig,
    issued_at,
    expires_at,
    now,
    "system"
))

# 2. Update browser_ai_install
cur.execute("""
    UPDATE browser_ai_install
    SET accepted_license_id = %s,
        accepted_revision = %s,
        last_authority_ok = %s
    WHERE id = 'primary';
""", (
    payload["license_id"],
    payload["revision"],
    now
))

conn.commit()
print("Successfully activated license in database!")

# Verify
cur.execute("SELECT license_id, client_name, server_hardware_id, max_seats, expires_at FROM browser_ai_license;")
print("Updated License in DB:", cur.fetchone())

cur.execute("SELECT id, install_id, accepted_license_id, accepted_revision FROM browser_ai_install;")
print("Updated Install in DB:", cur.fetchone())

cur.close()
conn.close()
