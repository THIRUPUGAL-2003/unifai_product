import re

with open('ui/app/workspace/browser-ai/page.tsx', 'r', encoding='utf-8') as f:
    content = f.read()

# 1. Update device cell in device management table
old_dev_pattern = re.compile(r'<TableCell className="text-sm truncate">\{agent\.username \|\| [^\}]+\}</TableCell>')
new_dev = '''<TableCell className="text-sm">
\t\t\t\t\t\t\t\t\t\t\t\t<div className="truncate font-medium" title={agent.ad_upn || agent.username || ""}>
\t\t\t\t\t\t\t\t\t\t\t\t\t{agent.ad_upn ? agent.ad_upn : agent.ad_domain ? `${agent.ad_domain}\\\\${agent.username}` : (agent.username || "—")}
\t\t\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t\t\t{agent.is_domain_joined ? (
\t\t\t\t\t\t\t\t\t\t\t\t\t<div className="text-[10px] text-emerald-400 font-normal">
\t\t\t\t\t\t\t\t\t\t\t\t\t\tAD: {agent.ad_domain}
\t\t\t\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t\t\t\t</TableCell>'''

m1 = old_dev_pattern.search(content)
if m1:
    content = content[:m1.start()] + new_dev + content[m1.end():]
    print("[1] Updated device table cell successfully!")
else:
    print("[1] Device table cell pattern not matched")

# 2. Update telemetry cell in agents fleet table
old_tel_pattern = re.compile(r'<div className="text-\[11px\] text-muted-foreground truncate" title=\{agent\.username \|\| [^\}]+\}>\s*\{agent\.username \? `user: \$\{agent\.username\}` : [^\}]+\}\s*</div>')
new_tel = '''{agent.is_domain_joined ? (
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t<div className="text-[11px] text-sky-400 font-medium truncate flex items-center gap-1" title={agent.ad_upn || `${agent.ad_domain}\\\\${agent.username}`}>
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t<span className="w-1.5 h-1.5 rounded-full bg-emerald-400 inline-block shrink-0" />
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t<span>{agent.ad_upn || `${agent.ad_domain}\\\\${agent.username}`}</span>
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t) : (
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t<div className="text-[11px] text-muted-foreground truncate" title={agent.username || ""}>
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t{agent.username ? `user: ${agent.username}` : "—"}
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t)}'''

m2 = old_tel_pattern.search(content)
if m2:
    content = content[:m2.start()] + new_tel + content[m2.end():]
    print("[2] Updated telemetry table cell successfully!")
else:
    print("[2] Telemetry cell pattern not matched")

# 3. Update prompt log cell in prompt logs table
old_log_pattern = re.compile(r'<div className="truncate text-xs text-muted-foreground" title=\{log\.agent_hostname \|\| log\.agent_id \|\| ""\}>\s*\{log\.agent_hostname \|\| log\.agent_id \|\| [^\}]+\}\s*</div>')
new_log = '''<div className="truncate text-xs text-foreground font-medium" title={log.ad_upn || log.domain_user || log.agent_hostname || log.agent_id || ""}>
\t\t\t\t\t\t\t\t\t\t\t\t\t\t{log.ad_upn || (log.domain_user ? log.domain_user : (log.agent_hostname || log.agent_id || "—"))}
\t\t\t\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t\t\t\t{log.ad_domain && (
\t\t\t\t\t\t\t\t\t\t\t\t\t\t<div className="text-[10px] text-sky-400 font-mono truncate">{log.ad_domain}</div>
\t\t\t\t\t\t\t\t\t\t\t\t\t)}'''

m3 = old_log_pattern.search(content)
if m3:
    content = content[:m3.start()] + new_log + content[m3.end():]
    print("[3] Updated prompt log table cell successfully!")
else:
    print("[3] Prompt log cell pattern not matched")

# 4. Add Active Directory GPO Startup Script Option to Network Deploy Dialog
gpo_target = '''						{/* Option 3: Manual Direct Silent Arguments */}'''
gpo_replacement = '''						{/* Option 4: Active Directory GPO Startup Script (Mass 1000+ Laptops) */}
						<div className="space-y-1.5 p-3 rounded-md bg-secondary/20 border border-border">
							<div className="flex items-center justify-between">
								<span className="font-semibold text-foreground flex items-center gap-1.5">
									<Building2 className="h-3.5 w-3.5 text-emerald-400" />
									Active Directory GPO Startup Script (1,000+ Domain Laptops)
								</span>
								<Button
									size="sm"
									variant="ghost"
									className="h-7 text-xs gap-1 text-emerald-400 hover:text-emerald-300"
									onClick={() => {
										const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost:8080";
										const script = `$InstallerUrl = "${origin}/api/browser-ai/setup/Gateway_Guard_Setup.exe"\\n$TempPath = "$env:TEMP\\\\Gateway_Guard_Setup.exe"\\nif (-not (Test-Path "C:\\\\Program Files\\\\Gateway Guard\\\\Gateway Guard.exe")) {\\n    Invoke-WebRequest -Uri $InstallerUrl -OutFile $TempPath -UseBasicParsing\\n    Start-Process $TempPath -ArgumentList "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART" -Wait\\n    Remove-Item -Force $TempPath -ErrorAction SilentlyContinue\\n}`;
										navigator.clipboard.writeText(script);
										setNetworkDeployCopied("gpo");
										setTimeout(() => setNetworkDeployCopied(""), 2500);
									}}
								>
									{networkDeployCopied === "gpo" ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
									{networkDeployCopied === "gpo" ? "Copied!" : "Copy GPO Script"}
								</Button>
							</div>
							<p className="text-[11px] text-muted-foreground">
								Link this in <strong>Active Directory Group Policy (gpmc.msc) &gt; Computer Configuration &gt; Windows Settings &gt; Scripts &gt; Startup</strong>. All 1,000 domain computers silently install on next boot without employee interaction.
							</p>
							<pre className="p-2.5 rounded bg-black/60 border border-border/80 font-mono text-[11px] text-emerald-300 overflow-x-auto whitespace-pre-wrap break-all">
								{`$InstallerUrl = "${typeof window !== "undefined" ? window.location.origin : "http://localhost:8080"}/api/browser-ai/setup/Gateway_Guard_Setup.exe"\\n$TempPath = "$env:TEMP\\\\Gateway_Guard_Setup.exe"\\nif (-not (Test-Path "C:\\\\Program Files\\\\Gateway Guard\\\\Gateway Guard.exe")) {\\n    Invoke-WebRequest -Uri $InstallerUrl -OutFile $TempPath -UseBasicParsing\\n    Start-Process $TempPath -ArgumentList "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART" -Wait\\n    Remove-Item -Force $TempPath -ErrorAction SilentlyContinue\\n}`}
							</pre>
						</div>

						{/* Option 3: Manual Direct Silent Arguments */}'''

if gpo_target in content:
    content = content.replace(gpo_target, gpo_replacement, 1)
    print("[4] Added Active Directory GPO option to Network Deploy Dialog!")
else:
    print("[4] GPO target pattern not matched")

with open('ui/app/workspace/browser-ai/page.tsx', 'w', encoding='utf-8') as f:
    f.write(content)

print("[OK] UI Page successfully updated!")
