import sys
import builtins
sys.path.insert(0, 'd:/unifai_project/apps/browser-guard/proxy/gateway_proxy_parts')

import config_caches_rules
for k in dir(config_caches_rules):
    if not k.startswith('__'):
        setattr(builtins, k, getattr(config_caches_rules, k))

import helpers_prompts
for k in dir(helpers_prompts):
    if not k.startswith('__'):
        setattr(builtins, k, getattr(helpers_prompts, k))

import uploads_detect
for k in dir(uploads_detect):
    if not k.startswith('__'):
        setattr(builtins, k, getattr(uploads_detect, k))

class MockHeaders(dict):
    def get(self, k, default=''):
        for key, val in self.items():
            if key.lower() == k.lower():
                return val
        return default

class MockRequest:
    headers = MockHeaders({'content-type': 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'})
    path = '/api/organizations/bdae9546-5469-4020-aa12-15ae2d8a3673/conversations/8f27e1cc-eb2f-4ff0-ae61-d7d8a549b8f0/wiggle/upload-file'
    method = 'POST'
    pretty_host = 'claude.ai'
    content = b'PK\x03\x04mock_bytes_here' * 20

class MockFlow:
    request = MockRequest()

is_up, reason = helpers_prompts.detect_file_upload(MockFlow(), '')
print("detect_file_upload -> is_upload:", is_up, "reason:", reason)

path_lower = MockRequest.path.lower().split("?", 1)[0]
is_upload_endpoint = helpers_prompts._path_looks_like_upload(path_lower)
print("_path_looks_like_upload ->", is_upload_endpoint)
