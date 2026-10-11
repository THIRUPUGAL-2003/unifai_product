import urllib.request
import json

# Let's check available providers and models from Gateway API
req = urllib.request.Request('http://127.0.0.1:8080/api/models')
try:
    with urllib.request.urlopen(req) as resp:
        data = json.loads(resp.read())
        print('Total models configured:', len(data.get('models', [])))
        for m in data.get('models', [])[:10]:
            print('Model:', m.get('model'), '| Provider:', m.get('provider'))
except Exception as e:
    print('Error getting models:', e)
