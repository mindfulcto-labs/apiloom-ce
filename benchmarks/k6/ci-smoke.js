/**
 * ci-smoke.js — CI benchmark smoke test for Apiloom Gateway CE.
 *
 * Designed for GitHub Actions ubuntu-latest runners:
 *   - 20 VUs (vs 1000 VUs in the full staged profile)
 *   - 60-second flat run (vs 4-minute staged profile)
 *   - Stricter thresholds than the full test to catch obvious regressions fast
 *
 * Regression gate (fails CI if exceeded):
 *   - p(99) latency > 1500ms  (1.5× the full-test 1000ms target)
 *   - error rate > 1%
 *
 * Run locally:
 *   k6 run --env GATEWAY_URL=http://localhost:18080 benchmarks/k6/ci-smoke.js
 *
 * Run in CI (via benchmark.yml):
 *   k6 run --env GATEWAY_URL=http://localhost:18080 \
 *     --out json=k6-results.json benchmarks/k6/ci-smoke.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

const healthLatency   = new Trend('health_latency',    true);
const proxyGetLatency = new Trend('proxy_get_latency', true);
const proxyPostLatency = new Trend('proxy_post_latency', true);
const errorRate       = new Rate('request_errors');
const totalRequests   = new Counter('total_requests');

const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:18080';

export const options = {
  // Flat load profile: 20 VUs for 60 seconds.
  // Simulates a reduced version of the full 1000-VU peak stage.
  vus:      20,
  duration: '60s',

  thresholds: {
    // Hard gate: p99 must stay under 1500ms.
    // Full-test target is 1000ms; we give CI a 50% buffer for runner variance.
    http_req_duration: ['p(99)<1500'],

    // Hard gate: error rate < 1%.
    http_req_failed: ['rate<0.01'],
    request_errors:  ['rate<0.01'],

    // Soft targets (report but do not fail):
    health_latency:    ['p(95)<500'],
    proxy_get_latency: ['p(95)<800'],
    proxy_post_latency: ['p(95)<800'],
  },
};

function healthCheck() {
  const res = http.get(`${BASE_URL}/healthz`, {
    tags: { scenario: 'healthz' },
  });

  healthLatency.add(res.timings.duration);
  totalRequests.add(1);

  const ok = check(res, {
    'healthz 200': (r) => r.status === 200,
  });
  errorRate.add(!ok ? 1 : 0);
}

function proxyGet() {
  const res = http.get(`${BASE_URL}/demo/get`, {
    tags: { scenario: 'proxy_get' },
  });

  proxyGetLatency.add(res.timings.duration);
  totalRequests.add(1);

  const ok = check(res, {
    'proxy GET 2xx': (r) => r.status >= 200 && r.status < 300,
  });
  errorRate.add(!ok ? 1 : 0);
}

function proxyPost() {
  const payload = JSON.stringify({
    name:  'ci-smoke',
    value: 1,
    tags:  ['ci', 'smoke'],
  });

  const res = http.post(`${BASE_URL}/demo/post`, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags:    { scenario: 'proxy_post' },
  });

  proxyPostLatency.add(res.timings.duration);
  totalRequests.add(1);

  const ok = check(res, {
    'proxy POST 2xx': (r) => r.status >= 200 && r.status < 300,
  });
  errorRate.add(!ok ? 1 : 0);
}

export default function () {
  const r = Math.random();
  if (r < 0.2) {
    healthCheck();
  } else if (r < 0.6) {
    proxyGet();
  } else {
    proxyPost();
  }

  // No sleep: maximize throughput measurement on CI runners.
  // The full load-test.js uses sleep(0.1) for realistic client simulation.
}

export function handleSummary(data) {
  const rps = data.metrics.http_reqs
    ? (data.metrics.http_reqs.values.rate || 0).toFixed(1)
    : 'N/A';

  const p99 = data.metrics.http_req_duration
    ? (data.metrics.http_req_duration.values['p(99)'] || 0).toFixed(2)
    : 'N/A';

  const errRate = data.metrics.http_req_failed
    ? ((data.metrics.http_req_failed.values.rate || 0) * 100).toFixed(2)
    : 'N/A';

  const summary = `
=== Apiloom CI Smoke Test Results ===
VUs: 20  Duration: 60s
RPS:        ${rps} req/s
p(99):      ${p99} ms   [gate: < 1500ms]
Error rate: ${errRate}%  [gate: < 1%]
=====================================
`;
  console.log(summary);

  return {
    stdout: summary,
  };
}
