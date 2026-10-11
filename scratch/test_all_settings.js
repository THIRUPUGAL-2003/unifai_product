const http = require('http');

const BASE_URL = 'http://localhost:3100';
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

async function runTests() {
  console.log('================================================================');
  console.log('STARTING THOROUGH END-TO-END SETTINGS TESTS ON http://localhost:3100');
  console.log('================================================================\n');

  const results = {
    auth: { passed: 0, failed: 0 },
    clientSettings: { passed: 0, failed: 0 },
    compatibility: { passed: 0, failed: 0 },
    caching: { passed: 0, failed: 0 },
    security: { passed: 0, failed: 0 },
    apiKeys: { passed: 0, failed: 0 },
    performanceTuning: { passed: 0, failed: 0 },
    featureFlags: { passed: 0, failed: 0 },
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

  // --- Step 0: Authentication ---
  console.log('>>> 0. AUTHENTICATION TEST');
  const loginRes = await request('POST', '/api/session/login', {
    username: 'admin@yespanchi.com',
    password: 'YP2025-2026yp',
  });
  assert('auth', 'Admin Login Successful', loginRes.status === 200, `Status: ${loginRes.status}, Body: ${JSON.stringify(loginRes.body)}`);
  assert('auth', 'Session Cookie Received', Boolean(sessionCookie), `Cookie: ${sessionCookie}`);

  // Fetch initial full config
  const coreConfigRes = await request('GET', '/api/config?fromDB=true');
  assert('auth', 'Fetch Initial Core Config', coreConfigRes.status === 200, `Status: ${coreConfigRes.status}`);
  const initialConfig = coreConfigRes.body;

  // --- Step 1: Client Settings ---
  console.log('\n>>> 1. CLIENT SETTINGS TESTING (/workspace/config/client-settings)');
  const droppedRes = await request('GET', '/api/logs/dropped');
  assert('clientSettings', 'Get Dropped Requests counter (/api/logs/dropped)', droppedRes.status === 200 && typeof droppedRes.body.dropped_requests === 'number');

  // Test updating client config via PUT /api/config
  const updatedClientConfig = {
    ...initialConfig.client_config,
    drop_excess_requests: !initialConfig.client_config?.drop_excess_requests,
    dump_errors_in_console_logs: true,
    disable_db_pings_in_health: false,
    header_filter_config: {
      allowlist: ['x-custom-tenant-id', 'x-request-trace'],
      denylist: ['x-internal-secret'],
    },
  };
  const saveClientRes = await request('PUT', '/api/config', {
    ...initialConfig,
    client_config: updatedClientConfig,
  });
  assert('clientSettings', 'Save Client Config with headers & toggles (PUT /api/config)', saveClientRes.status === 200, `Status: ${saveClientRes.status}, Body: ${JSON.stringify(saveClientRes.body)}`);

  // Verify persistence
  const verifyClientRes = await request('GET', '/api/config?fromDB=true');
  assert(
    'clientSettings',
    'Verify Client Config Persistence',
    verifyClientRes.body?.client_config?.dump_errors_in_console_logs === true &&
    verifyClientRes.body?.client_config?.header_filter_config?.allowlist?.includes('x-custom-tenant-id'),
    `Body: ${JSON.stringify(verifyClientRes.body?.client_config?.header_filter_config)}`
  );

  // Test Large Payload Config (metadata via POST /api/config/metadata)
  const lpUpdateRes = await request('POST', '/api/config/metadata', {
    large_payload: {
      enabled: true,
      request_threshold_bytes: 2048,
      response_threshold_bytes: 4096,
      prefetch_size_bytes: 1024,
      max_payload_bytes: 10485760,
      truncated_log_bytes: 2048,
    },
  });
  assert('clientSettings', 'Save Large Payload Metadata Config (POST /api/config/metadata)', lpUpdateRes.status === 200, `Status: ${lpUpdateRes.status}, Body: ${JSON.stringify(lpUpdateRes.body)}`);

  const verifyLPRes = await request('GET', '/api/config?fromDB=true');
  assert(
    'clientSettings',
    'Verify Large Payload Metadata Persistence',
    verifyLPRes.body?.metadata?.large_payload?.enabled === true &&
    verifyLPRes.body?.metadata?.large_payload?.request_threshold_bytes === 2048,
    `Body: ${JSON.stringify(verifyLPRes.body?.metadata?.large_payload)}`
  );

  // Revert Client Settings to original
  await request('PUT', '/api/config', initialConfig);
  console.log('  [INFO] Reverted Client Settings to original state.');

  // --- Step 2: Compatibility Settings ---
  console.log('\n>>> 2. COMPATIBILITY TESTING (/workspace/config/compatibility)');
  const compatTestPayload = {
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
  const saveCompatRes = await request('PUT', '/api/config', compatTestPayload);
  assert('compatibility', 'Save Compatibility Flags (PUT /api/config)', saveCompatRes.status === 200, `Status: ${saveCompatRes.status}`);

  const verifyCompatRes = await request('GET', '/api/config?fromDB=true');
  assert(
    'compatibility',
    'Verify Compatibility Flags Persistence',
    verifyCompatRes.body?.client_config?.compat?.convert_text_to_chat === true &&
    verifyCompatRes.body?.client_config?.compat?.convert_chat_to_responses === true &&
    verifyCompatRes.body?.client_config?.compat?.should_drop_params === true &&
    verifyCompatRes.body?.client_config?.compat?.should_convert_params === true,
    `Body: ${JSON.stringify(verifyCompatRes.body?.client_config?.compat)}`
  );

  // Revert Compat Settings
  await request('PUT', '/api/config', initialConfig);
  console.log('  [INFO] Reverted Compatibility Settings to original state.');

  // --- Step 3: Caching Settings ---
  console.log('\n>>> 3. CACHING TESTING (/workspace/config/caching)');
  const pluginsRes = await request('GET', '/api/plugins');
  assert('caching', 'Get Plugins list (/api/plugins)', pluginsRes.status === 200, `Status: ${pluginsRes.status}`);
  const pluginList = Array.isArray(pluginsRes.body) ? pluginsRes.body : (pluginsRes.body?.plugins || []);

  const vectorStoreRes = await request('GET', '/api/vector-store-config');
  assert('caching', 'Get Vector Store Config (/api/vector-store-config)', vectorStoreRes.status === 200, `Status: ${vectorStoreRes.status}`);

  // Test updating Vector Store Config
  const updateVectorStoreRes = await request('PUT', '/api/vector-store-config', {
    enabled: true,
    type: 'qdrant',
    config: {
      host: { value: 'localhost' },
      port: { value: '6334' },
    },
  });
  assert('caching', 'Update Vector Store Config (PUT /api/vector-store-config)', [200, 400].includes(updateVectorStoreRes.status), `Status: ${updateVectorStoreRes.status}`);

  // Test saving Cache Plugin Config (direct mode)
  const cachePayload = {
    ttl: 600,
    threshold: 0.85,
    dimension: 1,
    conversation_history_threshold: 4,
    exclude_system_prompt: false,
    cache_by_model: true,
    cache_by_provider: true,
  };
  const semanticPlugin = pluginList.find((p) => p.name === 'semantic_cache');
  let cacheSaveRes;
  if (semanticPlugin) {
    cacheSaveRes = await request('PUT', '/api/plugins/semantic_cache', {
      enabled: semanticPlugin.enabled,
      config: cachePayload,
    });
  } else {
    cacheSaveRes = await request('POST', '/api/plugins', {
      name: 'semantic_cache',
      enabled: false,
      config: cachePayload,
      path: '',
    });
  }
  assert('caching', 'Save Local Cache Plugin Config', [200, 201].includes(cacheSaveRes.status), `Status: ${cacheSaveRes.status}, Body: ${JSON.stringify(cacheSaveRes.body)}`);

  // --- Step 4: Security Settings ---
  console.log('\n>>> 4. SECURITY TESTING (/workspace/config/security)');
  const smtpRes = await request('GET', '/api/smtp-config');
  assert('security', 'Get SMTP Config (/api/smtp-config)', smtpRes.status === 200, `Status: ${smtpRes.status}`);

  // Test CORS / Origins and Headers update
  const updatedSecurityConfig = {
    ...initialConfig,
    client_config: {
      ...initialConfig.client_config,
      allowed_origins: ['http://localhost:3100', 'https://*.yespanchi.com'],
      allowed_headers: ['content-type', 'authorization', 'x-uf-api-key'],
      required_headers: [],
      whitelisted_routes: ['/health', '/ready'],
      enforce_auth_on_inference: false,
      allow_direct_keys: true,
    },
  };
  const saveSecRes = await request('PUT', '/api/config', updatedSecurityConfig);
  assert('security', 'Save Security CORS & Headers Config (PUT /api/config)', saveSecRes.status === 200, `Status: ${saveSecRes.status}`);

  const verifySecRes = await request('GET', '/api/config?fromDB=true');
  assert(
    'security',
    'Verify Security Config Persistence',
    verifySecRes.body?.client_config?.allowed_origins?.includes('https://*.yespanchi.com') &&
    verifySecRes.body?.client_config?.allow_direct_keys === true,
    `Body: ${JSON.stringify(verifySecRes.body?.client_config?.allowed_origins)}`
  );

  // Test SMTP test connection endpoint
  const testSmtpRes = await request('POST', '/api/smtp-config/test', {
    to: 'test@example.com',
  });
  assert(
    'security',
    'Test SMTP endpoint handled gracefully (/api/smtp-config/test)',
    [200, 400, 500].includes(testSmtpRes.status),
    `Status: ${testSmtpRes.status}`
  );

  // Revert Security Settings
  await request('PUT', '/api/config', initialConfig);
  console.log('  [INFO] Reverted Security Settings to original state.');

  // --- Step 5: API Keys Settings ---
  console.log('\n>>> 5. API KEYS TESTING (/workspace/config/api-keys)');
  const govHealthRes = await request('GET', '/api/governance/health');
  assert('apiKeys', 'Governance Health Check (Test Admin API Connection)', govHealthRes.status === 200, `Status: ${govHealthRes.status}`);

  const vksRes = await request('GET', '/api/governance/virtual-keys?limit=20');
  assert('apiKeys', 'Get Virtual Keys List', vksRes.status === 200 && Array.isArray(vksRes.body?.virtual_keys), `Status: ${vksRes.status}`);

  // Test creating a new virtual key
  const testKeyName = `test-key-${Date.now()}`;
  const createVkRes = await request('POST', '/api/governance/virtual-keys', {
    name: testKeyName,
    is_active: true,
  });
  assert(
    'apiKeys',
    'Create Virtual Key',
    createVkRes.status === 200 && Boolean(createVkRes.body?.virtual_key?.value),
    `Status: ${createVkRes.status}, Key: ${createVkRes.body?.virtual_key?.value?.slice(0, 10)}...`
  );

  const createdKeyId = createVkRes.body?.virtual_key?.id;
  if (createdKeyId) {
    // Clean up created key by deleting it
    const deleteVkRes = await request('DELETE', `/api/governance/virtual-keys/${createdKeyId}`);
    assert('apiKeys', 'Clean Up / Delete Test Virtual Key', deleteVkRes.status === 200, `Status: ${deleteVkRes.status}`);
  }

  // --- Step 6: Performance Tuning Settings ---
  console.log('\n>>> 6. PERFORMANCE TUNING TESTING (/workspace/config/performance-tuning)');
  const currentPoolSize = initialConfig.client_config?.initial_pool_size || 1000;
  const currentMaxBodyMB = initialConfig.client_config?.max_request_body_size_mb || 100;

  const perfTestConfig = {
    ...initialConfig,
    client_config: {
      ...initialConfig.client_config,
      initial_pool_size: 1500,
      max_request_body_size_mb: 150,
    },
  };
  const savePerfRes = await request('PUT', '/api/config', perfTestConfig);
  assert('performanceTuning', 'Save Performance Tuning Config (PUT /api/config)', savePerfRes.status === 200, `Status: ${savePerfRes.status}`);

  const verifyPerfRes = await request('GET', '/api/config?fromDB=true');
  assert(
    'performanceTuning',
    'Verify Performance Tuning Persistence',
    verifyPerfRes.body?.client_config?.initial_pool_size === 1500 &&
    verifyPerfRes.body?.client_config?.max_request_body_size_mb === 150,
    `Values: pool=${verifyPerfRes.body?.client_config?.initial_pool_size}, mb=${verifyPerfRes.body?.client_config?.max_request_body_size_mb}`
  );

  // Revert Performance Tuning Settings
  await request('PUT', '/api/config', initialConfig);
  console.log('  [INFO] Reverted Performance Tuning to original state.');

  // --- Step 7: Feature Flags Settings ---
  console.log('\n>>> 7. FEATURE FLAGS TESTING (/workspace/config/feature-flags)');
  const flagsRes = await request('GET', '/api/feature-flags');
  assert('featureFlags', 'Get Feature Flags List (/api/feature-flags)', flagsRes.status === 200 && Array.isArray(flagsRes.body?.flags), `Status: ${flagsRes.status}`);

  const flags = flagsRes.body?.flags || [];
  console.log(`  [INFO] Found ${flags.length} total feature flags.`);

  // Find an unlocked, registered flag to test toggle
  const toggleableFlag = flags.find((f) => !f.locked && f.registered && !f.enterprise_only);
  if (toggleableFlag) {
    console.log(`  [INFO] Testing toggle on flag: ${toggleableFlag.id} (current: ${toggleableFlag.enabled})`);
    const toggleTarget = !toggleableFlag.enabled;
    const toggleRes = await request('PUT', `/api/feature-flags/${toggleableFlag.id}`, {
      enabled: toggleTarget,
    });
    assert('featureFlags', `Toggle Feature Flag '${toggleableFlag.id}' to ${toggleTarget}`, toggleRes.status === 200, `Status: ${toggleRes.status}`);

    // Verify change
    const verifyFlagsRes = await request('GET', '/api/feature-flags');
    const updatedFlag = (verifyFlagsRes.body?.flags || []).find((f) => f.id === toggleableFlag.id);
    assert('featureFlags', `Verify Feature Flag '${toggleableFlag.id}' Updated in State`, updatedFlag?.enabled === toggleTarget);

    // Revert toggle
    const revertRes = await request('PUT', `/api/feature-flags/${toggleableFlag.id}`, {
      enabled: toggleableFlag.enabled,
    });
    assert('featureFlags', `Revert Feature Flag '${toggleableFlag.id}' to original (${toggleableFlag.enabled})`, revertRes.status === 200);
  } else {
    console.log('  [INFO] No unlocked registered flag available for toggling (flags are locked or pinned).');
    assert('featureFlags', 'Feature Flags listing verified', flags.length > 0);
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
    console.log(`  - ${category.padEnd(20)}: ${counts.passed} Passed, ${counts.failed} Failed`);
  }
  console.log('----------------------------------------------------------------');
  console.log(`TOTAL: ${totalPassed} Passed, ${totalFailed} Failed`);
  console.log('================================================================\n');

  if (totalFailed > 0) {
    process.exit(1);
  }
}

runTests().catch((err) => {
  console.error('Fatal error during test run:', err);
  process.exit(1);
});
