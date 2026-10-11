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

import file_policy
for k in dir(file_policy):
    if not k.startswith('__'):
        setattr(builtins, k, getattr(file_policy, k))

url_path = '/api/organizations/bdae9546-5469-4020-aa12-15ae2d8a3673/conversations/8f27e1cc-eb2f-4ff0-ae61-d7d8a549b8f0/wiggle/upload-file'
print("helpers_prompts._path_looks_like_upload:", helpers_prompts._path_looks_like_upload(url_path))
print("uploads_detect._file_policy_applies_on_send:", uploads_detect._file_policy_applies_on_send(url_path, 'PK...'))
