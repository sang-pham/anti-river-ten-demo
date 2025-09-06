import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

// Configuration via environment variables
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const IDENTIFIER = __ENV.IDENTIFIER || 'admin@example.com';
const PASSWORD = __ENV.PASSWORD || 'admin123';

// Test options (override with: k6 run -e VUS=50 -e DURATION=2m k6/login_test.js)
export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: __ENV.DURATION || '1m',
  thresholds: {
    http_req_failed: ['rate<0.01'],      // <1% requests failed
    http_req_duration: ['p(95)<500'],    // 95% under 500ms
    checks: ['rate>0.99'],               // >99% checks pass
  },
};

const loginDuration = new Trend('login_duration_ms');
const loginFailures = new Counter('login_failures');

export default function () {
  const url = `${BASE_URL}/v1/auth/login`;
  const payload = JSON.stringify({
    identifier: IDENTIFIER,
    password: PASSWORD,
  });
  const params = {
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    },
    timeout: '30s',
  };

  const res = http.post(url, payload, params);

  // Add timing metric
  loginDuration.add(res.timings.duration);

  // Validate response
  const ok = check(res, {
    'status is 200': (r) => r.status === 200,
    'has token field': (r) => {
      try {
        const t = r.json('token');
        return typeof t === 'string' && t.length > 0;
      } catch {
        return false;
      }
    },
  });

  if (!ok) {
    loginFailures.add(1);
  }

  // Pace iterations
  sleep(1);
}
