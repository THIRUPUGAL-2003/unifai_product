const http = require('http');

const BASE_URL = 'http://localhost:3101';
let sessionCookie = '';

function request(method, path, body = null, headers = {}) {
  return new Promise((resolve, reject) => {
    const url = new URL(path, BASE_URL);
    const options = {
      hostname: url.hostname,
      port: url.port,
      path: url.pathname + url.search,
      method: method,
      headers: {
        'Content-Type': 'application/json',
        ...headers,
      },
    };
    if (sessionCookie) {
      options.headers['Cookie'] = sessionCookie;
    }

    const req = http.request(options, (res) => {
      let data = '';
      if (res.headers['set-cookie']) {
        const cookies = Array.isArray(res.headers['set-cookie'])
          ? res.headers['set-cookie']
          : [res.headers['set-cookie']];
        for (const c of cookies) {
          const cookiePart = c.split(';')[0];
          if (cookiePart.includes('session=') || cookiePart.includes('token=')) {
            sessionCookie = cookiePart;
          }
        }
        if (!sessionCookie && cookies.length > 0) {
          sessionCookie = cookies[0].split(';')[0];
        }
      }
      res.on('data', (chunk) => (data += chunk));
      res.on('end', () => {
        let parsed = data;
        try {
          parsed = JSON.parse(data);
        } catch (e) {}
        resolve({ status: res.statusCode, headers: res.headers, body: parsed });
      });
    });

    req.on('error', reject);
    if (body) {
      req.write(typeof body === 'string' ? body : JSON.stringify(body));
    }
    req.end();
  });
}

async function runAllTests() {
  console.log('================================================================');
  console.log('COMPREHENSIVE TESTING ON ' + BASE_URL);
  console.log('Testing: Prompt Repo, Skills Repo & All 7 Settings Sections');
  console.log('================================================================\n');

  const results = {
    spa_routes: { passed: 0, failed: 0 },
    auth: { passed: 0, failed: 0 },
    prompt_repo: { passed: 0, failed: 0 },
    skills_repo: { passed: 0, failed: 0 },
    client_settings: { passed: 0, failed: 0 },
    compatibility: { passed: 0, failed: 0 },
    caching: { passed: 0, failed: 0 },
    security: { passed: 0, failed: 0 },
    api_keys: { passed: 0, failed: 0 },
    performance_tuning: { passed: 0, failed: 0 },
    feature_flags: { passed: 0, failed: 0 },
  };

  function assert(category, testName, condition, details = '') {
    if (condition) {
      results[category].passed++;
      console.log(`  [PASS] ${testName}`);
    } else {
      results[category].failed++;
      console.error(`  [FAIL] ${testName}: ${details}`);
    }
  }

  // --- Step 0: Route check on UI (SPA) ---
  console.log('>>> 0. SPA PAGE ROUTES ACCESSIBILITY');
  const pages = [
    '/workspace/prompt-repo',
    '/workspace/skills-repo',
    '/workspace/config/client-settings',
    '/workspace/config/compatibility',
    '/workspace/config/caching',
    '/workspace/config/security',
    '/workspace/config/api-keys',
    '/workspace/config/performance-tuning',
    '/workspace/config/feature-flags',
  ];
  for (const p of pages) {
    const res = await request('GET', p);
    assert('spa_routes', `Page HTTP 200: ${p}`, res.status === 200, `Status: ${res.status}`);
  }

  // --- Step 1: Authentication ---
  console.log('\n>>> 1. AUTHENTICATION & SESSION');
  const loginRes = await request('POST', '/api/session/login', {
    username: 'admin@yespanchi.com',
    password: 'YP2025-2026yp',
  });
  assert('auth', 'Admin Login Successful', loginRes.status === 200, `Status: ${loginRes.status}`);
  assert('auth', 'Session Cookie Obtained', Boolean(sessionCookie), `Cookie: ${sessionCookie}`);

  // Fetch initial config for later restore
  const initialConfigRes = await request('GET', '/api/config?fromDB=true');
  const initialConfig = initialConfigRes.body || {};

  // --- Step 2: Prompt Repository ---
  console.log('\n>>> 2. PROMPT REPOSITORY (/workspace/prompt-repo)');
  const listPromptsRes = await request('GET', '/api/prompt-repo/prompts');
  assert('prompt_repo', 'List Prompts (/api/prompt-repo/prompts)', listPromptsRes.status === 200, `Status: ${listPromptsRes.status}`);

  const testPromptPayload = {
    name: `Test Prompt ${Date.now()}`,
  };
  const createPromptRes = await request('POST', '/api/prompt-repo/prompts', testPromptPayload);
  assert('prompt_repo', 'Create New Prompt (POST /api/prompt-repo/prompts)', [200, 201].includes(createPromptRes.status), `Status: ${createPromptRes.status}, Body: ${JSON.stringify(createPromptRes.body)}`);

  const createdPromptId = createPromptRes.body?.id || createPromptRes.body?.prompt?.id;
  if (createdPromptId) {
    const getPromptRes = await request('GET', `/api/prompt-repo/prompts/${createdPromptId}`);
    assert('prompt_repo', `Get Created Prompt (GET /api/prompt-repo/prompts/${createdPromptId})`, getPromptRes.status === 200, `Status: ${getPromptRes.status}`);

    const updatePromptRes = await request('PUT', `/api/prompt-repo/prompts/${createdPromptId}`, {
      name: `${testPromptPayload.name} (Updated)`,
    });
    assert('prompt_repo', `Update Prompt (PUT /api/prompt-repo/prompts/${createdPromptId})`, [200, 204].includes(updatePromptRes.status), `Status: ${updatePromptRes.status}`);

    const deletePromptRes = await request('DELETE', `/api/prompt-repo/prompts/${createdPromptId}`);
    assert('prompt_repo', `Delete Prompt (DELETE /api/prompt-repo/prompts/${createdPromptId})`, [200, 204].includes(deletePromptRes.status), `Status: ${deletePromptRes.status}`);
  }

  // --- Step 3: Skills Repository ---
  console.log('\n>>> 3. SKILLS REPOSITORY (/workspace/skills-repo)');
  const listSkillsRes = await request('GET', '/api/skills');
  assert('skills_repo', 'List Skills (/api/skills)', listSkillsRes.status === 200, `Status: ${listSkillsRes.status}`);

  const skillsVersionRes = await request('GET', '/api/skills/all/version');
  assert('skills_repo', 'Get All Skills Version (/api/skills/all/version)', skillsVersionRes.status === 200, `Status: ${skillsVersionRes.status}, Body: ${JSON.stringify(skillsVersionRes.body)}`);

  // Test Codex Git Smart HTTP endpoint
  const gitInfoRefsRes = await request('GET', '/api/skills/serve/codex/info/refs?service=git-upload-pack');
  assert('skills_repo', 'Codex Git Smart HTTP Info/Refs (/api/skills/serve/codex/info/refs)', [200, 404].includes(gitInfoRefsRes.status), `Status: ${gitInfoRefsRes.status}`);

  // Test creating a skill
  const skillName = `test-skill-${Date.now()}`;
  const testSkillPayload = {
    name: skillName,
    description: 'Automated test skill verification',
    version: '1.0.0',
    skill_md_body: `---\nname: ${skillName}\ndescription: Automated test skill\n---\n# Test Skill Instructions\nExecution details.`,
  };
  const createSkillRes = await request('POST', '/api/skills', testSkillPayload);
  assert('skills_repo', 'Create Skill (POST /api/skills)', [200, 201].includes(createSkillRes.status), `Status: ${createSkillRes.status}, Body: ${JSON.stringify(createSkillRes.body)}`);

  const createdSkillId = createSkillRes.body?.id || createSkillRes.body?.skill?.id;
  if (createdSkillId) {
    const getSkillRes = await request('GET', `/api/skills/${createdSkillId}`);
    assert('skills_repo', `Get Created Skill (GET /api/skills/${createdSkillId})`, getSkillRes.status === 200);

    const deleteSkillRes = await request('DELETE', `/api/skills/${createdSkillId}`);
    assert('skills_repo', `Clean Up Test Skill (DELETE /api/skills/${createdSkillId})`, [200, 204].includes(deleteSkillRes.status));
  }

  // --- Step 4: Client Settings ---
  console.log('\n>>> 4. CLIENT SETTINGS (/workspace/config/client-settings)');
  const droppedRes = await request('GET', '/api/logs/dropped');
  assert('client_settings', 'Get Dropped Requests (/api/logs/dropped)', droppedRes.status === 200 && typeof droppedRes.body?.dropped_requests === 'number');

  const updatedClient = {
    ...initialConfig.client_config,
    drop_excess_requests: false,
    dump_errors_in_console_logs: true,
    header_filter_config: {
      allowlist: ['x-request-id', 'x-trace-id'],
      denylist: ['authorization-internal'],
    },
  };
  const saveClientRes = await request('PUT', '/api/config', {
    ...initialConfig,
    client_config: updatedClient,
  });
  assert('client_settings', 'Save Client Settings (PUT /api/config)', saveClientRes.status === 200, `Status: ${saveClientRes.status}`);

  const verifyClientRes = await request('GET', '/api/config?fromDB=true');
  assert('client_settings', 'Verify Client Settings Persistence', verifyClientRes.body?.client_config?.dump_errors_in_console_logs === true);

  // Large Payload
  const lpRes = await request('POST', '/api/config/metadata', {
    large_payload: {
      enabled: true,
      request_threshold_bytes: 4096,
      response_threshold_bytes: 8192,
      prefetch_size_bytes: 2048,
      max_payload_bytes: 10485760,
      truncated_log_bytes: 4096,
    },
  });
  assert('client_settings', 'Save Large Payload Metadata (POST /api/config/metadata)', lpRes.status === 200, `Status: ${lpRes.status}`);

  await request('PUT', '/api/config', initialConfig);

  // --- Step 5: Compatibility ---
  console.log('\n>>> 5. COMPATIBILITY (/workspace/config/compatibility)');
  const compatPayload = {
    ...initialConfig,
    client_config: {
      ...initialConfig.client_config,
      compat: {
        convert_text_to_chat: true,
        convert_chat_to_responses: true,
        should_drop_params: true,
        should_convert_params: true,
      },
    },
  };
  const saveCompatRes = await request('PUT', '/api/config', compatPayload);
  assert('compatibility', 'Save Compatibility Settings (PUT /api/config)', saveCompatRes.status === 200, `Status: ${saveCompatRes.status}`);

  const verifyCompatRes = await request('GET', '/api/config?fromDB=true');
  assert('compatibility', 'Verify Compatibility Persistence', verifyCompatRes.body?.client_config?.compat?.convert_text_to_chat === true);

  await request('PUT', '/api/config', initialConfig);

  // --- Step 6: Caching ---
  console.log('\n>>> 6. CACHING (/workspace/config/caching)');
  const pluginsRes = await request('GET', '/api/plugins');
  assert('caching', 'Get Plugins List (/api/plugins)', pluginsRes.status === 200, `Status: ${pluginsRes.status}`);

  const vectorStoreRes = await request('GET', '/api/vector-store-config');
  assert('caching', 'Get Vector Store Config (/api/vector-store-config)', vectorStoreRes.status === 200, `Status: ${vectorStoreRes.status}`);

  // --- Step 7: Security ---
  console.log('\n>>> 7. SECURITY (/workspace/config/security)');
  const smtpRes = await request('GET', '/api/smtp-config');
  assert('security', 'Get SMTP Config (/api/smtp-config)', smtpRes.status === 200, `Status: ${smtpRes.status}`);

  const secConfig = {
    ...initialConfig,
    client_config: {
      ...initialConfig.client_config,
      allowed_origins: ['http://localhost:3101', 'https://*.yespanchi.com'],
      allowed_headers: ['content-type', 'authorization'],
      whitelisted_routes: ['/health'],
    },
  };
  const saveSecRes = await request('PUT', '/api/config', secConfig);
  assert('security', 'Save Security Config (PUT /api/config)', saveSecRes.status === 200, `Status: ${saveSecRes.status}`);

  await request('PUT', '/api/config', initialConfig);

  // --- Step 8: API Keys ---
  console.log('\n>>> 8. API KEYS (/workspace/config/api-keys)');
  const govRes = await request('GET', '/api/governance/health');
  assert('api_keys', 'Governance Health Check (/api/governance/health)', govRes.status === 200, `Status: ${govRes.status}`);

  const vksRes = await request('GET', '/api/governance/virtual-keys?limit=20');
  assert('api_keys', 'List Virtual Keys (/api/governance/virtual-keys)', vksRes.status === 200 && Array.isArray(vksRes.body?.virtual_keys), `Status: ${vksRes.status}`);

  // --- Step 9: Performance Tuning ---
  console.log('\n>>> 9. PERFORMANCE TUNING (/workspace/config/performance-tuning)');
  const perfConfig = {
    ...initialConfig,
    client_config: {
      ...initialConfig.client_config,
      initial_pool_size: 1200,
      max_request_body_size_mb: 120,
    },
  };
  const savePerfRes = await request('PUT', '/api/config', perfConfig);
  assert('performance_tuning', 'Save Performance Tuning Config (PUT /api/config)', savePerfRes.status === 200, `Status: ${savePerfRes.status}`);

  const verifyPerfRes = await request('GET', '/api/config?fromDB=true');
  assert('performance_tuning', 'Verify Performance Tuning Persistence', verifyPerfRes.body?.client_config?.initial_pool_size === 1200);

  await request('PUT', '/api/config', initialConfig);

  // --- Step 10: Feature Flags ---
  console.log('\n>>> 10. FEATURE FLAGS (/workspace/config/feature-flags)');
  const flagsRes = await request('GET', '/api/feature-flags');
  assert('feature_flags', 'List Feature Flags (/api/feature-flags)', flagsRes.status === 200 && Array.isArray(flagsRes.body?.flags), `Status: ${flagsRes.status}`);

  const flags = flagsRes.body?.flags || [];
  console.log(`  [INFO] Total registered feature flags: ${flags.length}`);
  const sectionFlags = flags.filter(f => f.id.startsWith('section.'));
  const productFlags = flags.filter(f => f.id.startsWith('browser.') || f.id.startsWith('cache.'));
  console.log(`  [INFO] Section flags found: ${sectionFlags.length} (${sectionFlags.map(f => f.id).join(', ')})`);
  console.log(`  [INFO] Product behavior flags found: ${productFlags.length} (${productFlags.map(f => f.id).join(', ')})`);

  assert('feature_flags', 'Section flags exist (e.g. section.prompt-repository, section.skills-repository)', sectionFlags.length >= 8);
  assert('feature_flags', 'Product behavior flags exist (browser.prompt_guard, browser.file_scan, cache.responses)', productFlags.length >= 3);

  // Test toggling a flag
  const toggleable = flags.find(f => !f.locked && f.registered && !f.enterprise_only);
  if (toggleable) {
    const origState = toggleable.enabled;
    const targetState = !origState;
    console.log(`  [INFO] Testing toggle flag '${toggleable.id}' from ${origState} to ${targetState}...`);
    const toggleRes = await request('PUT', `/api/feature-flags/${toggleable.id}`, { enabled: targetState });
    assert('feature_flags', `Toggle flag '${toggleable.id}' to ${targetState}`, toggleRes.status === 200, `Status: ${toggleRes.status}`);

    const verifyToggleRes = await request('GET', '/api/feature-flags');
    const updatedFlag = (verifyToggleRes.body?.flags || []).find(f => f.id === toggleable.id);
    assert('feature_flags', `Verify updated state for '${toggleable.id}'`, updatedFlag?.enabled === targetState);

    // Revert back
    await request('PUT', `/api/feature-flags/${toggleable.id}`, { enabled: origState });
    console.log(`  [INFO] Reverted flag '${toggleable.id}' back to ${origState}`);
  }

  // --- Final Summary ---
  console.log('\n================================================================');
  console.log('SUMMARY OF ALL TEST RESULTS:');
  console.log('================================================================');
  let totalPassed = 0;
  let totalFailed = 0;
  for (const [category, counts] of Object.entries(results)) {
    totalPassed += counts.passed;
    totalFailed += counts.failed;
    console.log(`  - ${category.padEnd(22)}: ${counts.passed} Passed, ${counts.failed} Failed`);
  }
  console.log('----------------------------------------------------------------');
  console.log(`TOTAL: ${totalPassed} Passed, ${totalFailed} Failed`);
  console.log('================================================================\n');

  if (totalFailed > 0) {
    process.exit(1);
  }
}

runAllTests().catch((err) => {
  console.error('Fatal error during test run:', err);
  process.exit(1);
});
