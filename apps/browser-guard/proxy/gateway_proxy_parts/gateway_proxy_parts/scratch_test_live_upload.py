import sys
sys.path.insert(0, r'd:\unifai_project\apps\browser-guard\proxy')
import browser_ai_proxy as p
from unittest.mock import MagicMock
import json

p.clear_domain_upload_caches('chatgpt.com')
p.clear_domain_upload_caches('oaiusercontent.com')

# Simulate 3 files uploaded to files.oaiusercontent.com
def make_flow(url_path, ctype, body):
    f = MagicMock()
    f.request.pretty_host = 'files.oaiusercontent.com'
    f.request.path = url_path
    f.request.method = 'POST'
    f.request.url = 'https://files.oaiusercontent.com' + url_path
    f.request.headers = {'content-type': ctype, 'referer': 'https://chatgpt.com/'}
    f.request.content = body
    f.request.stream = False
    f.metadata = {}
    return f

flow1 = make_flow('/file-AAA111', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', b'PK\x03\x04' + b'excel data' * 20)
flow2 = make_flow('/file-BBB222', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document', b'PK\x03\x04' + b'docx data' * 20)
flow3 = make_flow('/file-CCC333', 'text/plain', b'Confidential text content with pincode 613002' * 5)

addon = p.BrowserAIInterceptor()
print("Uploading file 1...")
addon.request(flow1)
print("Uploading file 2...")
addon.request(flow2)
print("Uploading file 3...")
addon.request(flow3)

chatgpt_send = {
    "action": "next",
    "messages": [
        {
            "id": "aaa-111",
            "author": {"role": "user"},
            "content": {
                "content_type": "multimodal_text",
                "parts": [
                    "Can you summarize these files for me?",
                    {"asset_pointer": "file-service://file-AAA111", "size_bytes": 204},
                    {"asset_pointer": "file-service://file-BBB222", "size_bytes": 184},
                    {"asset_pointer": "file-service://file-CCC333", "size_bytes": 225}
                ]
            },
            "metadata": {
                "attachments": [
                    {"id": "file-AAA111", "name": "financial_report.xlsx", "size": 204, "mime_type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
                    {"id": "file-BBB222", "name": "passwords_list.docx", "size": 184, "mime_type": "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
                    {"id": "file-CCC333", "name": "project_notes.txt", "size": 225, "mime_type": "text/plain"}
                ]
            }
        }
    ],
    "parent_message_id": "root",
    "model": "gpt-4o"
}

raw_send = json.dumps(chatgpt_send)

flow_send = MagicMock()
flow_send.request.pretty_host = 'chatgpt.com'
flow_send.request.path = '/backend-api/conversation'
flow_send.request.method = 'POST'
flow_send.request.url = 'https://chatgpt.com/backend-api/conversation'
flow_send.request.headers = {'content-type': 'application/json', 'referer': 'https://chatgpt.com/'}
flow_send.request.content = raw_send.encode('utf-8')
flow_send.request.stream = False
flow_send.metadata = {}

print("\nSending chatgpt conversation...")
addon.request(flow_send)
