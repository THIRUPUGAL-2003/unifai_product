"""Open each kept website through the local Guard proxy and record the result."""
import ssl
import urllib.request

PROXY = "http://127.0.0.1:18103"
SITES = [
    ("ChatGPT", "https://chatgpt.com"),
    ("Claude", "https://claude.ai"),
    ("Gemini", "https://gemini.google.com"),
    ("DeepSeek", "https://chat.deepseek.com"),
    ("Copilot", "https://copilot.microsoft.com"),
    ("Perplexity", "https://www.perplexity.ai"),
    ("Mistral", "https://chat.mistral.ai"),
    ("Hugging Face", "https://huggingface.co/chat"),
    ("Lovable", "https://lovable.dev"),
    ("Grok", "https://grok.com"),
    ("Meta AI", "https://www.meta.ai"),
    ("Poe", "https://poe.com"),
    ("Character.ai", "https://character.ai"),
    ("Cursor", "https://cursor.com"),
    ("Codeium", "https://windsurf.ai"),
    ("Midjourney", "https://www.midjourney.com"),
    ("Leonardo AI", "https://leonardo.ai"),
    ("Runway", "https://runwayml.com"),
    ("ElevenLabs", "https://elevenlabs.io"),
    ("Suno", "https://suno.com"),
    ("Stability AI", "https://stability.ai"),
    ("Groq", "https://groq.com"),
    ("OpenRouter", "https://openrouter.ai"),
    ("Together AI", "https://www.together.ai"),
    ("Fireworks AI", "https://fireworks.ai"),
    ("Replicate", "https://replicate.com"),
    ("Cohere", "https://cohere.com"),
    ("Gamma", "https://gamma.app"),
    ("Jasper", "https://www.jasper.ai"),
    ("Notion AI", "https://www.notion.so"),
    ("Writesonic", "https://writesonic.com"),
    ("Copy.ai", "https://www.copy.ai"),
    ("You.com", "https://you.com"),
    ("Phind", "https://www.phind.com"),
    ("Pi", "https://pi.ai"),
    ("Blackbox AI", "https://www.blackbox.ai"),
    ("V0 by Vercel", "https://v0.dev"),
    ("StackBlitz Bolt", "https://bolt.new"),
    ("Kling AI", "https://klingai.com"),
    ("Sora", "https://sora.com"),
    ("Adobe Firefly", "https://firefly.adobe.com"),
    ("Krea AI", "https://www.krea.ai"),
    ("Luma AI", "https://lumalabs.ai"),
    ("Pika Labs", "https://pika.art"),
    ("DuckDuckGo AI", "https://duck.ai"),
    ("Brave Leo", "https://leo.brave.com"),
    ("Grammarly", "https://www.grammarly.com"),
    ("DeepL", "https://www.deepl.com"),
    ("Quillbot", "https://quillbot.com"),
    ("Civitai", "https://civitai.com"),
]

proxy = urllib.request.ProxyHandler({"http": PROXY, "https": PROXY})
ctx = ssl._create_unverified_context()
opener = urllib.request.build_opener(proxy, urllib.request.HTTPSHandler(context=ctx))

ok = 0
for name, url in SITES:
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    try:
        with opener.open(req, timeout=12) as resp:
            code = resp.status
            final = resp.geturl()
        print(f"OPEN {code} {name} {final[:80]}")
        ok += 1
    except Exception as e:
        msg = str(e).splitlines()[0][:140]
        print(f"FAIL {name} {msg}")
print(f"opened {ok}/{len(SITES)}")
