import re
import json

raw = '{"prompt": "explain this code", "attachments": [{"file_name": "app.py", "extracted_content": "def foo():\\n    return {\\"pin\\": 600028}\\n", "file_type": "text/x-python"}]}'

pattern = r'\{[^{}]*?"(?:extracted_content|extractedContent)"\s*:\s*"((?:[^"\\]|\\.)+)"[^{}]*?\}'

matches = list(re.finditer(pattern, raw))
print("Regex matches with curly braces in content:", len(matches))
if matches:
    print("Match 0:", matches[0].group(0)[:80])
