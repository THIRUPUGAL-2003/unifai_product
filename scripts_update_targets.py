import psycopg2
import json
import datetime

TARGETS = [
    # ── ChatGPT ──────────────────────────────────────────────────
    {
        'id': 'tgt-chatgpt-root',
        'domain': 'chatgpt.com',
        'platform_name': 'ChatGPT',
        'host_role': 'ui',
        'parent_id': '',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-ab',
        'domain': 'ab.chatgpt.com',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-chat-openai',
        'domain': 'chat.openai.com',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-api-openai',
        'domain': 'api.openai.com',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-files-oai',
        'domain': 'files.oaiusercontent.com',
        'platform_name': 'ChatGPT',
        'host_role': 'file',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-oaiusercontent',
        'domain': 'oaiusercontent.com',
        'platform_name': 'ChatGPT',
        'host_role': 'file',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-oaistatic',
        'domain': 'oaistatic.com',
        'platform_name': 'ChatGPT',
        'host_role': 'file',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-cdn-oaistatic',
        'domain': 'cdn.oaistatic.com',
        'platform_name': 'ChatGPT',
        'host_role': 'file',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-ws',
        'domain': 'ws.chatgpt.com',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-ws-api',
        'domain': 'ws-api.chatgpt.com',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-realtime',
        'domain': 'realtime.chatgpt.com',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-openai-com',
        'domain': 'openai.com',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-chatgpt-livekit',
        'domain': 'browser-gateway-openai.livekit.cloud',
        'platform_name': 'ChatGPT',
        'host_role': 'chat',
        'parent_id': 'tgt-chatgpt-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },

    # ── Claude ───────────────────────────────────────────────────
    {
        'id': 'tgt-claude-root',
        'domain': 'claude.ai',
        'platform_name': 'Claude',
        'host_role': 'ui',
        'parent_id': '',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-claude-api',
        'domain': 'api.anthropic.com',
        'platform_name': 'Claude',
        'host_role': 'chat',
        'parent_id': 'tgt-claude-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-claude-files',
        'domain': 'files.claudeusercontent.com',
        'platform_name': 'Claude',
        'host_role': 'file',
        'parent_id': 'tgt-claude-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-claude-cdn',
        'domain': 'cdn.claude.ai',
        'platform_name': 'Claude',
        'host_role': 'file',
        'parent_id': 'tgt-claude-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-claude-anthropic-com',
        'domain': 'anthropic.com',
        'platform_name': 'Claude',
        'host_role': 'ui',
        'parent_id': 'tgt-claude-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },

    # ── Gemini ───────────────────────────────────────────────────
    {
        'id': 'tgt-gemini-root',
        'domain': 'gemini.google.com',
        'platform_name': 'Gemini',
        'host_role': 'ui',
        'parent_id': '',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-gemini-clients6',
        'domain': 'clients6.google.com',
        'platform_name': 'Gemini',
        'host_role': 'chat',
        'parent_id': 'tgt-gemini-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-gemini-generativelanguage',
        'domain': 'generativelanguage.googleapis.com',
        'platform_name': 'Gemini',
        'host_role': 'chat',
        'parent_id': 'tgt-gemini-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-gemini-aistudio',
        'domain': 'aistudio.google.com',
        'platform_name': 'Gemini',
        'host_role': 'ui',
        'parent_id': 'tgt-gemini-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
    {
        'id': 'tgt-gemini-bard',
        'domain': 'bard.google.com',
        'platform_name': 'Gemini',
        'host_role': 'ui',
        'parent_id': 'tgt-gemini-root',
        'monitored': True,
        'block_site': False,
        'status': 'MONITORED',
    },
]

def update_db():
    print("Connecting to database...")
    conn = psycopg2.connect(
        host='76.13.243.253',
        port=32768,
        dbname='Unifai_test',
        user='Unifai_test',
        password='YP2025-2026yp',
        sslmode='disable'
    )
    cur = conn.cursor()

    # 1. Backup existing records
    cur.execute("SELECT id, domain, platform_name, host_role, monitored, block_site, intercepted_count, status, reply_bot_enabled, reply_bot_provider, reply_bot_model, reply_bot_mode, parent_id, created_at FROM browser_target_websites;")
    existing = cur.fetchall()
    print(f"Current records count in DB: {len(existing)}")
    with open("d:/unifai_project/browser_target_websites_backup_latest.json", "w", encoding="utf-8") as f:
        json.dump([str(x) for x in existing], f, indent=2)

    # 2. Clear table
    print("Clearing browser_target_websites table...")
    cur.execute("DELETE FROM browser_target_websites;")
    print(f"Cleared rows.")

    # 3. Insert cleanly structured targets
    now = datetime.datetime.now(datetime.timezone.utc)
    insert_sql = """
    INSERT INTO browser_target_websites (
        id, domain, platform_name, host_role, monitored, block_site,
        intercepted_count, status, reply_bot_enabled, reply_bot_provider,
        reply_bot_model, reply_bot_mode, parent_id, created_at
    ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
    """

    for t in TARGETS:
        cur.execute(insert_sql, (
            t['id'],
            t['domain'],
            t['platform_name'],
            t['host_role'],
            t['monitored'],
            t['block_site'],
            0,
            t['status'],
            False,
            '',
            '',
            '',
            t['parent_id'],
            now
        ))

    conn.commit()
    print(f"Successfully inserted {len(TARGETS)} new clean target records.")

    # 4. Verify
    cur.execute("SELECT id, domain, platform_name, host_role, monitored, parent_id FROM browser_target_websites ORDER BY platform_name, parent_id, domain;")
    rows = cur.fetchall()
    print("\nVerified records in DB:")
    for r in rows:
        parent_indicator = f"-> sub of {r[5]}" if r[5] else "[PARENT]"
        print(f"  {r[2]:8} | {r[1]:38} | role: {r[3]:4} | {parent_indicator}")

    cur.close()
    conn.close()

if __name__ == '__main__':
    update_db()
