import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics per scenario.
const healthLatency = new Trend('health_latency', true);
const proxyGetLatency = new Trend('proxy_get_latency', true);
const proxyPostLatency = new Trend('proxy_post_latency', true);
const errorRate = new Rate('request_errors');

// Configurable via environment variables.
const BASE_URL = __ENV.GATEWAY_URL || 'http://localhost:18080';

export const options = {
  stages: [
    { duration: '30s', target: 100 },   // ramp up
    { duration: '2m',  target: 500 },   // sustained load
    { duration: '1m',  target: 1000 },  // peak load
    { duration: '30s', target: 0 },     // ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<30', 'p(99)<50'],
    http_req_failed:   ['rate<0.01'],
    request_errors:    ['rate<0.01'],
  },
};

// Scenario 1: Health check baseline.
// Measures gateway overhead without proxying to a backend.
function healthCheck() {
  const res = http.get(`${BASE_URL}/healthz`, {
    tags: { scenario: 'healthz' },
  });

  healthLatency.add(res.timings.duration);

  const ok = check(res, {
    'healthz status 200': (r) => r.status === 200,
  });

  if (!ok) {
    errorRate.add(1);
  } else {
    errorRate.add(0);
  }
}

// Scenario 2: Proxied GET request.
// Measures end-to-end latency through the gateway to a backend.
function proxyGet() {
  const res = http.get(`${BASE_URL}/demo/get`, {
    tags: { scenario: 'proxy_get' },
  });

  proxyGetLatency.add(res.timings.duration);

  const ok = check(res, {
    'proxy GET status 2xx': (r) => r.status >= 200 && r.status < 300,
  });

  if (!ok) {
    errorRate.add(1);
  } else {
    errorRate.add(0);
  }
}

// Scenario 3: Proxied POST with JSON body.
// Measures proxy overhead with request body forwarding.
function proxyPost() {
  const payload = JSON.stringify({
    name: 'benchmark-test',
    value: 42,
    tags: ['load', 'test'],
  });

  const params = {
    headers: { 'Content-Type': 'application/json' },
    tags: { scenario: 'proxy_post' },
  };

  const res = http.post(`${BASE_URL}/demo/post`, payload, params);

  proxyPostLatency.add(res.timings.duration);

  const ok = check(res, {
    'proxy POST status 2xx': (r) => r.status >= 200 && r.status < 300,
  });

  if (!ok) {
    errorRate.add(1);
  } else {
    errorRate.add(0);
  }
}

// Default function distributes load across all scenarios.
export default function () {
  const rand = Math.random();

  if (rand < 0.2) {
    healthCheck();
  } else if (rand < 0.6) {
    proxyGet();
  } else {
    proxyPost();
  }

  // Small think time to simulate realistic client behavior.
  sleep(0.1);
}
