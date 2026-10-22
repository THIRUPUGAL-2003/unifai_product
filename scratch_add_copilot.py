import psycopg2
import uuid
import datetime

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()
cur.execute("SELECT domain FROM browser_target_websites WHERE domain IN ('copilot.com', 'www.copilot.com')")
existing = [r[0] for r in cur.fetchall()]
print('Existing:', existing)

sql = """
INSERT INTO browser_target_websites (
    id, domain, platform_name, monitored, block_site, intercepted_count,
    status, reply_bot_enabled, reply_bot_provider, reply_bot_model,
    reply_bot_mode, parent_id, created_at, host_role
) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
"""

if 'copilot.com' not in existing:
    cur.execute(sql, (
        'tgt-' + uuid.uuid4().hex[:8], 'copilot.com', 'Copilot', True, False, 0,
        'MONITORED', False, '', '', '', 'tgt-df3d0ba5', datetime.datetime.now(datetime.timezone.utc), 'ui'
    ))
    print('Inserted copilot.com')

if 'www.copilot.com' not in existing:
    cur.execute(sql, (
        'tgt-' + uuid.uuid4().hex[:8], 'www.copilot.com', 'Copilot', True, False, 0,
        'MONITORED', False, '', '', '', 'tgt-df3d0ba5', datetime.datetime.now(datetime.timezone.utc), 'ui'
    ))
    print('Inserted www.copilot.com')

conn.commit()
cur.execute("SELECT domain, platform_name, monitored, status, host_role FROM browser_target_websites WHERE platform_name='Copilot' ORDER BY domain")
for r in cur.fetchall():
    print(r)
