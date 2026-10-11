import psycopg2

KEEP = [
    "ChatGPT",
    "Claude",
    "Gemini",
    "DeepSeek",
    "Copilot",
    "Perplexity",
    "Mistral",
    "Hugging Face",
    "Lovable",
    "Grok",
    "Meta AI",
    "Poe",
    "Character.ai",
    "Cursor",
    "Codeium",
    "Midjourney",
    "Leonardo AI",
    "Runway",
    "ElevenLabs",
    "Suno",
    "Stability AI",
    "Groq",
    "OpenRouter",
    "Together AI",
    "Fireworks AI",
    "Replicate",
    "Cohere",
    "Gamma",
    "Jasper",
    "Notion AI",
    "Writesonic",
    "Copy.ai",
    "You.com",
    "Phind",
    "Pi",
    "Blackbox AI",
    "V0 by Vercel",
    "StackBlitz Bolt",
    "Kling AI",
    "Sora",
    "Adobe Firefly",
    "Krea AI",
    "Luma AI",
    "Pika Labs",
    "DuckDuckGo AI",
    "Brave Leo",
    "Grammarly",
    "DeepL",
    "Quillbot",
    "Civitai",
]

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
cur.execute(
    "UPDATE browser_target_websites SET platform_name = 'OpenRouter' WHERE platform_name = 'OpenRouter AI Service'"
)
print("renamed openrouter", cur.rowcount)
cur.execute(
    "DELETE FROM browser_target_websites WHERE platform_name <> ALL(%s)",
    (KEEP,),
)
print("deleted", cur.rowcount)
cur.execute(
    """
    SELECT platform_name, count(*)
    FROM browser_target_websites
    GROUP BY platform_name
    ORDER BY platform_name
    """
)
rows = cur.fetchall()
names = [n for n, _ in rows]
missing = [n for n in KEEP if n not in names]
extra = [n for n in names if n not in KEEP]
print("platforms", len(rows), "rows", sum(n for _, n in rows))
print("missing", missing)
print("extra", extra)
for name, n in rows:
    print(f"{n}\t{name}")
if missing or extra or len(rows) != 50:
    conn.rollback()
    raise SystemExit("not exactly the 50 platforms; rolled back")
conn.commit()
print("committed")
conn.close()
