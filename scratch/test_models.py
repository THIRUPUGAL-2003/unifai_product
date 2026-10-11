from db_config import get_base_url
base_url = get_base_url()
import urllib.request
import json

# Let's check available providers and models from Gateway API
req = urllib.request.Request(f'{base_url}/api/models')
try:
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read())
        print('Total models configured:', len(data.get('models', [])))
        for m in data.get('models', [])[:10]:
            print('Model:', m.get('model'), '| Provider:', m.get('provider'))
except Exception as e:
    print('Error getting models:', e)
