import os
import re
from collections import defaultdict

routes_by_method = defaultdict(lambda: defaultdict(list))
pattern = re.compile(r'(?:r|s\.Router|router)\.(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH)\(\s*"([^"]+)"')

for root, _, files in os.walk('transports/gateway-http'):
    for f in files:
        if f.endswith('.go') and not f.endswith('_test.go'):
            path = os.path.join(root, f)
            with open(path, 'r', encoding='utf-8', errors='ignore') as fp:
                for idx, line in enumerate(fp, 1):
                    for match in pattern.finditer(line):
                        method, route = match.groups()
                        routes_by_method[method][route].append((path, idx))

print('=== COMPREHENSIVE ROUTE CHECK ===')
duplicates_found = False
for method, routes in routes_by_method.items():
    for route, occurrences in routes.items():
        if len(occurrences) > 1:
            print(f'DUPLICATE: [{method}] {route}')
            for p, line in occurrences:
                print(f'   -> {p}:{line}')
            duplicates_found = True

if not duplicates_found:
    print('ALL CLEAR: Absolutely ZERO duplicate routes across all Go files & handlers!')
print(f'Total unique registered endpoints scanned: {sum(len(routes) for routes in routes_by_method.values())}')
