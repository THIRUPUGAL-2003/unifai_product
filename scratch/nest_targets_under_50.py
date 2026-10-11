import psycopg2

PRIMARY = {
    "ChatGPT": "chatgpt.com",
    "Claude": "claude.ai",
    "Gemini": "gemini.google.com",
    "DeepSeek": "chat.deepseek.com",
    "Copilot": "copilot.microsoft.com",
    "Perplexity": "perplexity.ai",
    "Mistral": "chat.mistral.ai",
    "Hugging Face": "huggingface.co",
    "Lovable": "lovable.dev",
    "Grok": "grok.com",
    "Meta AI": "meta.ai",
    "Poe": "poe.com",
    "Character.ai": "character.ai",
    "Cursor": "cursor.com",
    "Codeium": "windsurf.ai",
    "Midjourney": "midjourney.com",
    "Leonardo AI": "leonardo.ai",
    "Runway": "runwayml.com",
    "ElevenLabs": "elevenlabs.io",
    "Suno": "suno.com",
    "Stability AI": "stability.ai",
    "Groq": "groq.com",
    "OpenRouter": "openrouter.ai",
    "Together AI": "together.ai",
    "Fireworks AI": "fireworks.ai",
    "Replicate": "replicate.com",
    "Cohere": "cohere.com",
    "Gamma": "gamma.app",
    "Jasper": "jasper.ai",
    "Notion AI": "notion.so",
    "Writesonic": "writesonic.com",
    "Copy.ai": "copy.ai",
    "You.com": "you.com",
    "Phind": "phind.com",
    "Pi": "pi.ai",
    "Blackbox AI": "blackbox.ai",
    "V0 by Vercel": "v0.dev",
    "StackBlitz Bolt": "bolt.new",
    "Kling AI": "klingai.com",
    "Sora": "sora.com",
    "Adobe Firefly": "firefly.adobe.com",
    "Krea AI": "krea.ai",
    "Luma AI": "lumalabs.ai",
    "Pika Labs": "pika.art",
    "DuckDuckGo AI": "duck.ai",
    "Brave Leo": "leo.brave.com",
    "Grammarly": "grammarly.com",
    "DeepL": "deepl.com",
    "Quillbot": "quillbot.com",
    "Civitai": "civitai.com",
}

env_vars = {}
with open("d:/unifai_project/.env", "r", encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            env_vars[k.strip()] = v.strip().strip("'\"")

conn = psycopg2.connect(
    dbname=env_vars.get("DB_NAME", "unifai"),
    user=env_vars.get("DB_USER", "postgres"),
    password=env_vars.get("DB_PASSWORD", "postgres"),
    host=env_vars.get("DB_HOST", "localhost"),
    port=env_vars.get("DB_PORT", "5432"),
)
conn.autocommit = False
cur = conn.cursor()
cur.execute("SELECT id, platform_name, domain, host_role FROM browser_target_websites")
rows = cur.fetchall()
by_platform = {}
for rid, platform, domain, role in rows:
    by_platform.setdefault(platform, []).append((rid, domain, role or ""))

def pick(platform, hosts):
    want = PRIMARY.get(platform, "")
    for rid, domain, role in hosts:
        if domain == want:
            return rid, domain
    ui = [(rid, domain) for rid, domain, role in hosts if role == "ui" and not domain.startswith(("api.", "cdn."))]
    pool = ui or [(rid, domain) for rid, domain, _ in hosts]
    pool.sort(key=lambda item: (item[1].count("."), len(item[1]), item[1]))
    return pool[0]

updates = 0
roots = []
for platform, hosts in sorted(by_platform.items()):
    root_id, root_domain = pick(platform, hosts)
    cur.execute(
        "UPDATE browser_target_websites SET parent_id = '' WHERE id = %s",
        (root_id,),
    )
    cur.execute(
        "UPDATE browser_target_websites SET parent_id = %s WHERE platform_name = %s AND id <> %s",
        (root_id, platform, root_id),
    )
    updates += cur.rowcount
    roots.append((platform, root_domain, 1 + cur.rowcount))

cur.execute(
    "SELECT count(*) FROM browser_target_websites WHERE parent_id IS NULL OR parent_id = ''"
)
top = cur.fetchone()[0]
print("top_level", top, "nested_updates", updates)
if top != 50:
    conn.rollback()
    raise SystemExit("expected 50 top-level websites")
for platform, domain, n in roots:
    print(f"{domain}\t{platform}\t{n}")
conn.commit()
print("committed")
conn.close()
