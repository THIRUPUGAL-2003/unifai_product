import psycopg2
import uuid

conn = psycopg2.connect(
    host='76.13.243.253',
    port=32768,
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    sslmode='disable'
)
cur = conn.cursor()

# Get chatgpt.com id
cur.execute("SELECT id FROM browser_target_websites WHERE domain = 'chatgpt.com'")
row = cur.fetchone()
chatgpt_id = row[0] if row else ""

# Get perplexity.ai id
cur.execute("SELECT id FROM browser_target_websites WHERE domain = 'perplexity.ai'")
row = cur.fetchone()
perplexity_id = row[0] if row else ""

# Add openai.com if not exists
cur.execute("SELECT id FROM browser_target_websites WHERE domain = 'openai.com'")
if not cur.fetchone() and chatgpt_id:
    cur.execute(
        "INSERT INTO browser_target_websites (id, domain, platform_name, host_role, parent_id, monitored, block_site, status) "
        "VALUES (%s, %s, %s, %s, %s, %s, %s, %s)",
        ('tgt-' + str(uuid.uuid4())[:8], 'openai.com', 'ChatGPT', 'ui', chatgpt_id, True, False, 'active')
    )
    print("Added openai.com under ChatGPT")

# Add www.perplexity.ai if not exists
cur.execute("SELECT id FROM browser_target_websites WHERE domain = 'www.perplexity.ai'")
if not cur.fetchone() and perplexity_id:
    cur.execute(
        "INSERT INTO browser_target_websites (id, domain, platform_name, host_role, parent_id, monitored, block_site, status) "
        "VALUES (%s, %s, %s, %s, %s, %s, %s, %s)",
        ('tgt-' + str(uuid.uuid4())[:8], 'www.perplexity.ai', 'Perplexity', 'ui', perplexity_id, True, False, 'active')
    )
    print("Added www.perplexity.ai under Perplexity")

conn.commit()

# Print full breakdown
cur.execute("SELECT domain, platform_name, host_role, parent_id, monitored FROM browser_target_websites ORDER BY platform_name, domain")
all_rows = cur.fetchall()
print(f"\nTotal targets in database: {len(all_rows)}")
for r in all_rows:
    print(f"[{r[1]}] {r[0]} (Role: {r[2]}, Parent: {r[3] or 'ROOT'})")

cur.close()
conn.close()
