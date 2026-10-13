import subprocess
import socket
import json
import urllib.request
import os

def check_port(port):
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(1.0)
        return s.connect_ex(('127.0.0.1', port)) == 0

print("Port 18103 (mitmproxy):", check_port(18103))
print("Port 18195 (PAC):", check_port(18195))
print("Port 8080 (backend):", check_port(8080))

# Check processes
cmd = 'tasklist /fi "imagename eq Gateway_Guard.exe"'
res = subprocess.run(cmd, capture_output=True, text=True, shell=True)
print("Processes:")
print(res.stdout)
