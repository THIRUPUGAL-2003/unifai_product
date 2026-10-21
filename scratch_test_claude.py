import re, json

raw = '{"prompt": "check this code", "attachments": [{"file_name": "test.js", "file_type": "application/javascript", "extracted_content": "function test() {\\n  return { success: true };\\n}"}]}'

pattern = r'\{[^{}]*?"(?:extracted_content|extractedContent)"\s*:\s*"((?:[^"\\]|\\.)+)"[^{}]*?\}'
matches = list(re.finditer(pattern, raw))
print("Regex match count:", len(matches))
for m in matches:
    print("Block:", m.group(0))
    print("Group 1:", m.group(1))
