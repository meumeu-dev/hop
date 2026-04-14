// Cloudflare Worker — hop pairing relay + accounts
// Stockage temporaire chiffré, expire après 2 min (pairing)
// Comptes: le worker ne peut JAMAIS lire les données machines (chiffrées côté client)

const MAX_SESSIONS_PER_ACCOUNT = 3;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;

    // No CORS by default — CLI and Android don't need it
    // Only pairing endpoints get minimal CORS for web dashboard compatibility
    const noCors = {};
    const pairingCors = {
      "Access-Control-Allow-Origin": request.headers.get("Origin") || "",
      "Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type, X-Pair-Token",
    };

    if (request.method === "OPTIONS") {
      // Only allow CORS preflight for pairing endpoints
      if (path.startsWith("/pair")) {
        return new Response(null, { headers: pairingCors });
      }
      return new Response(null, { status: 204 });
    }

    // Determine which CORS headers to use
    const cors = path.startsWith("/pair") ? pairingCors : noCors;

    // ==================== ACCOUNTS ====================

    // POST /auth/register — create account (random salt stored server-side)
    if (path === "/auth/register" && request.method === "POST") {
      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      const { email, username, auth_hash } = body;

      if (!email || !username || !auth_hash) {
        return jsonResponse({ error: "missing fields" }, 400, cors);
      }

      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
        return jsonResponse({ error: "invalid email" }, 400, cors);
      }

      if (!/^[a-zA-Z0-9_-]{2,32}$/.test(username)) {
        return jsonResponse({ error: "invalid username" }, 400, cors);
      }

      // Rate limit: max 10 registrations per IP per 15 min
      const ip = request.headers.get("CF-Connecting-IP") || "unknown";
      const rlKey = `ratelimit:register:${ip}`;
      const rlCount = parseInt(await env.HOP_KV.get(rlKey) || "0");
      if (rlCount >= 10) {
        return jsonResponse({ error: "trop de tentatives, reessaie plus tard" }, 429, cors);
      }
      await env.HOP_KV.put(rlKey, String(rlCount + 1), { expirationTtl: 900 });

      // Check if email/username already taken — block if account exists with valid auth data
      const existingEmailId = await env.HOP_KV.get(`account:email:${email.toLowerCase()}`);
      if (existingEmailId) {
        const acctData = await env.HOP_KV.get(`account:${existingEmailId}`);
        if (acctData) {
          try {
            const acct = JSON.parse(acctData);
            if (acct.serverHash) {
              return jsonResponse({ error: "inscription impossible" }, 409, cors);
            }
          } catch {}
        }
        // Index points to non-existent or invalid account — stale, allow overwrite
      }

      const existingUserId = await env.HOP_KV.get(`account:user:${username.toLowerCase()}`);
      if (existingUserId && existingUserId !== existingEmailId) {
        const acctData = await env.HOP_KV.get(`account:${existingUserId}`);
        if (acctData) {
          try {
            const acct = JSON.parse(acctData);
            if (acct.serverHash) {
              return jsonResponse({ error: "inscription impossible" }, 409, cors);
            }
          } catch {}
        }
      }

      const accountId = crypto.randomUUID();

      // Use client-provided salt or generate one (client sends random salt at registration)
      let authSalt = body.auth_salt;
      if (!authSalt || !/^[0-9a-f]{32}$/.test(authSalt)) {
        const saltBytes = new Uint8Array(16);
        crypto.getRandomValues(saltBytes);
        authSalt = Array.from(saltBytes).map(b => b.toString(16).padStart(2, '0')).join('');
      }

      // auth_hash is already computed by client with this salt — hash again server-side
      const serverHash = await sha256(auth_hash);

      const sessionToken = generateToken();
      const sessionHash = await sha256(sessionToken);

      const account = {
        id: accountId,
        email: email.toLowerCase(),
        username: username.toLowerCase(),
        serverHash: serverHash,
        authSalt: authSalt,
        created: Date.now(),
      };

      await env.HOP_KV.put(`account:${accountId}`, JSON.stringify(account));
      await env.HOP_KV.put(`account:email:${email.toLowerCase()}`, accountId);
      await env.HOP_KV.put(`account:user:${username.toLowerCase()}`, accountId);

      // Store session (7 days)
      await env.HOP_KV.put(`session:${sessionHash}`, JSON.stringify({
        accountId: accountId,
        created: Date.now(),
      }), { expirationTtl: 7 * 86400 });

      // Track session for this account
      await addAccountSession(env, accountId, sessionHash);

      return jsonResponse({
        ok: true,
        account_id: accountId,
        username: username,
        token: sessionToken,
      }, 200, cors);
    }

    // GET /auth/salt?email=xxx — returns the random salt for an email (step 1 of login)
    if (path === "/auth/salt" && request.method === "GET") {
      const email = url.searchParams.get("email");
      if (!email) {
        return jsonResponse({ error: "missing email" }, 400, cors);
      }

      // Rate limit
      const ip = request.headers.get("CF-Connecting-IP") || "unknown";
      const rlKey = `ratelimit:salt:${ip}`;
      const rlCount = parseInt(await env.HOP_KV.get(rlKey) || "0");
      if (rlCount >= 20) {
        return jsonResponse({ error: "trop de tentatives" }, 429, cors);
      }
      await env.HOP_KV.put(rlKey, String(rlCount + 1), { expirationTtl: 900 });

      const fakeSalt = async () => {
        const h = await sha256("fake-salt:" + email.toLowerCase());
        return jsonResponse({ salt: h.slice(0, 32) }, 200, cors);
      };

      // Support lookup by email OR username
      let accountId;
      if (email.includes("@")) {
        accountId = await env.HOP_KV.get(`account:email:${email.toLowerCase()}`);
      } else {
        accountId = await env.HOP_KV.get(`account:user:${email.toLowerCase()}`);
      }
      if (!accountId || accountId === "" || accountId === "DELETED") {
        return await fakeSalt();
      }

      const accountData = await env.HOP_KV.get(`account:${accountId}`);
      if (!accountData) {
        // Stale index — clean up the correct key
        if (email.includes("@")) {
          await env.HOP_KV.delete(`account:email:${email.toLowerCase()}`);
        } else {
          await env.HOP_KV.delete(`account:user:${email.toLowerCase()}`);
        }
        return await fakeSalt();
      }

      const account = JSON.parse(accountData);
      if (!account.authSalt) {
        return await fakeSalt();
      }
      return jsonResponse({ salt: account.authSalt }, 200, cors);
    }

    // POST /auth/login — authenticate (step 2: client computed hash with real salt)
    if (path === "/auth/login" && request.method === "POST") {
      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      const { email, auth_hash } = body;

      if (!email || !auth_hash) {
        return jsonResponse({ error: "missing fields" }, 400, cors);
      }

      // Rate limit per IP
      const ip = request.headers.get("CF-Connecting-IP") || "unknown";
      const rlKey = `ratelimit:login:${ip}`;
      const rlCount = parseInt(await env.HOP_KV.get(rlKey) || "0");
      if (rlCount >= 10) {
        return jsonResponse({ error: "trop de tentatives, reessaie plus tard" }, 429, cors);
      }
      await env.HOP_KV.put(rlKey, String(rlCount + 1), { expirationTtl: 900 });

      // Support login by email OR username
      let accountId;
      if (email.includes("@")) {
        accountId = await env.HOP_KV.get(`account:email:${email.toLowerCase()}`);
      } else {
        accountId = await env.HOP_KV.get(`account:user:${email.toLowerCase()}`);
      }
      if (!accountId || accountId === "" || accountId === "DELETED") {
        return jsonResponse({ error: "identifiants invalides" }, 401, cors);
      }

      // Rate limit per account (prevents bypass via email/username alternation)
      const acctRlKey = `ratelimit:login:acct:${accountId}`;
      const acctRlCount = parseInt(await env.HOP_KV.get(acctRlKey) || "0");
      if (acctRlCount >= 5) {
        return jsonResponse({ error: "trop de tentatives, reessaie plus tard" }, 429, cors);
      }
      await env.HOP_KV.put(acctRlKey, String(acctRlCount + 1), { expirationTtl: 900 });

      const accountData = await env.HOP_KV.get(`account:${accountId}`);
      if (!accountData) {
        return jsonResponse({ error: "identifiants invalides" }, 401, cors);
      }

      const account = JSON.parse(accountData);
      const serverHash = await sha256(auth_hash);

      if (serverHash !== account.serverHash) {
        return jsonResponse({ error: "identifiants invalides" }, 401, cors);
      }

      const sessionToken = generateToken();
      const sessionHash = await sha256(sessionToken);

      await env.HOP_KV.put(`session:${sessionHash}`, JSON.stringify({
        accountId: accountId,
        created: Date.now(),
      }), { expirationTtl: 7 * 86400 });

      // Track + enforce max sessions
      await addAccountSession(env, accountId, sessionHash);

      return jsonResponse({
        ok: true,
        account_id: accountId,
        username: account.username,
        email: account.email,
        token: sessionToken,
      }, 200, cors);
    }

    // GET /account/machines
    if (path === "/account/machines" && request.method === "GET") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, cors);

      const machinesData = await env.HOP_KV.get(`machines:${auth.accountId}`);
      const machines = machinesData ? JSON.parse(machinesData) : {};

      return jsonResponse({ ok: true, machines: machines }, 200, cors);
    }

    // PUT /account/machines
    if (path === "/account/machines" && request.method === "PUT") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, cors);

      const contentLength = parseInt(request.headers.get("Content-Length") || "0");
      if (contentLength > 1048576) {
        return jsonResponse({ error: "payload too large" }, 413, cors);
      }

      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      if (!body.data || typeof body.data !== "string" || body.data.length > 1048576) {
        return jsonResponse({ error: "invalid data" }, 400, cors);
      }

      await env.HOP_KV.put(`machines:${auth.accountId}`, JSON.stringify(body.data));
      return jsonResponse({ ok: true }, 200, cors);
    }

    // POST /auth/logout
    if (path === "/auth/logout" && request.method === "POST") {
      const token = extractBearerToken(request);
      if (token) {
        const sessionHash = await sha256(token);
        await env.HOP_KV.delete(`session:${sessionHash}`);
      }
      return jsonResponse({ ok: true }, 200, cors);
    }

    // DELETE /account — delete account and all data
    if (path === "/account" && request.method === "DELETE") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, cors);

      const accountData = await env.HOP_KV.get(`account:${auth.accountId}`);
      if (accountData) {
        const account = JSON.parse(accountData);
        // Remove all indexes
        await env.HOP_KV.delete(`account:email:${account.email}`);
        await env.HOP_KV.delete(`account:user:${account.username}`);
        // Remove sessions
        const sessionsData = await env.HOP_KV.get(`sessions:${auth.accountId}`);
        if (sessionsData) {
          const sessions = JSON.parse(sessionsData);
          for (const sh of sessions) {
            await env.HOP_KV.delete(`session:${sh}`);
          }
          await env.HOP_KV.delete(`sessions:${auth.accountId}`);
        }
      }
      // Remove account + machines
      await env.HOP_KV.delete(`account:${auth.accountId}`);
      await env.HOP_KV.delete(`machines:${auth.accountId}`);

      return jsonResponse({ ok: true }, 200, cors);
    }

    // ==================== PAIRING (v3 — short code) ====================
    // Pairing uses an 8-char alphanumeric code [a-z0-9]{8} shared by both sides.
    // The code is BOTH the lookup key AND the AES-GCM encryption key (via Argon2id
    // client-side). The worker never sees cleartext. Security budget:
    //   - 36^8 ≈ 2.8e12 combinations
    //   - TTL 120s
    //   - Rate-limit: 60 req/IP/min on pair endpoints
    //   - Up to 5 responses per code (first-valid-decrypt wins client-side)

    const codeRe = /^[a-z0-9]{8}$/;

    // Rate-limit helper: 60 req/min per IP on pairing paths
    async function pairRateLimit() {
      const ip = request.headers.get("CF-Connecting-IP") || "unknown";
      const rlKey = `rl:pair:${ip}`;
      const n = parseInt(await env.HOP_KV.get(rlKey) || "0");
      if (n >= 60) return true;
      await env.HOP_KV.put(rlKey, String(n + 1), { expirationTtl: 60 });
      return false;
    }

    // POST /pair  body: {code, data} — create session
    if (path === "/pair" && request.method === "POST") {
      if (await pairRateLimit()) return jsonResponse({ error: "rate limit" }, 429, cors);
      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      const { code, data } = body || {};
      if (!code || !codeRe.test(code)) return jsonResponse({ error: "invalid code" }, 400, cors);
      if (!data || typeof data !== "string" || data.length > 32768) {
        return jsonResponse({ error: "invalid data" }, 400, cors);
      }

      const key = `pair:${code}`;
      if (await env.HOP_KV.get(key)) {
        return jsonResponse({ error: "code already in use" }, 409, cors);
      }
      await env.HOP_KV.put(key, JSON.stringify({ data, created: Date.now(), respCount: 0 }),
        { expirationTtl: 120 });
      return jsonResponse({ ok: true, expires_in: 120 }, 200, cors);
    }

    // GET /pair/<code> — fetch encrypted session data
    const mPair = path.match(/^\/pair\/([a-z0-9]{8})$/);
    if (mPair && request.method === "GET") {
      if (await pairRateLimit()) return jsonResponse({ error: "rate limit" }, 429, cors);
      const stored = await env.HOP_KV.get(`pair:${mPair[1]}`);
      if (!stored) return jsonResponse({ error: "not found or expired" }, 404, cors);
      return jsonResponse({ data: JSON.parse(stored).data }, 200, cors);
    }

    // DELETE /pair/<code> — cleanup (best-effort, TTL handles it anyway)
    if (mPair && request.method === "DELETE") {
      if (await pairRateLimit()) return jsonResponse({ error: "rate limit" }, 429, cors);
      await env.HOP_KV.delete(`pair:${mPair[1]}`);
      for (let i = 0; i < 5; i++) await env.HOP_KV.delete(`pair:${mPair[1]}:r:${i}`);
      return jsonResponse({ ok: true }, 200, cors);
    }

    // POST /pair/<code>/response  body: {data} — deposit an encrypted response
    const mResp = path.match(/^\/pair\/([a-z0-9]{8})\/response$/);
    if (mResp && request.method === "POST") {
      if (await pairRateLimit()) return jsonResponse({ error: "rate limit" }, 429, cors);
      const code = mResp[1];
      const stored = await env.HOP_KV.get(`pair:${code}`);
      if (!stored) return jsonResponse({ error: "not found" }, 404, cors);
      const parsed = JSON.parse(stored);
      if (parsed.respCount >= 5) return jsonResponse({ error: "too many responses" }, 429, cors);

      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      if (!body.data || typeof body.data !== "string" || body.data.length > 32768) {
        return jsonResponse({ error: "invalid data" }, 400, cors);
      }

      const idx = parsed.respCount;
      await env.HOP_KV.put(`pair:${code}:r:${idx}`, body.data, { expirationTtl: 120 });
      parsed.respCount = idx + 1;
      await env.HOP_KV.put(`pair:${code}`, JSON.stringify(parsed), { expirationTtl: 120 });

      return jsonResponse({ ok: true, idx }, 200, cors);
    }

    // GET /pair/<code>/response?idx=N — fetch Nth response (0..4)
    if (mResp && request.method === "GET") {
      if (await pairRateLimit()) return jsonResponse({ error: "rate limit" }, 429, cors);
      const code = mResp[1];
      const idxStr = url.searchParams.get("idx") || "0";
      const idx = parseInt(idxStr);
      if (isNaN(idx) || idx < 0 || idx > 4) {
        return jsonResponse({ error: "invalid idx" }, 400, cors);
      }
      const stored = await env.HOP_KV.get(`pair:${code}:r:${idx}`);
      if (!stored) return jsonResponse({ error: "no response" }, 404, cors);
      return jsonResponse({ data: stored }, 200, cors);
    }

    if (path === "/health") {
      return jsonResponse({ status: "ok", service: "hop-pair" }, 200, cors);
    }

    return jsonResponse({ error: "not found" }, 404, cors);
  },
};

// ==================== HELPERS ====================

async function authenticateRequest(request, env) {
  const token = extractBearerToken(request);
  if (!token) return null;

  const sessionHash = await sha256(token);
  const sessionData = await env.HOP_KV.get(`session:${sessionHash}`);
  if (!sessionData) return null;

  return JSON.parse(sessionData);
}

function extractBearerToken(request) {
  const auth = request.headers.get("Authorization");
  if (!auth || !auth.startsWith("Bearer ")) return null;
  return auth.slice(7);
}

function generateToken() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('');
}

// Track sessions per account, enforce max limit
async function addAccountSession(env, accountId, sessionHash) {
  const key = `sessions:${accountId}`;
  let sessions = [];
  const existing = await env.HOP_KV.get(key);
  if (existing) {
    sessions = JSON.parse(existing);
  }

  sessions.push(sessionHash);

  // Evict oldest sessions if over limit
  while (sessions.length > MAX_SESSIONS_PER_ACCOUNT) {
    const oldest = sessions.shift();
    await env.HOP_KV.delete(`session:${oldest}`);
  }

  await env.HOP_KV.put(key, JSON.stringify(sessions));
}

async function sha256(message) {
  const msgBuffer = new TextEncoder().encode(message);
  const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
}

function jsonResponse(data, status, headers = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      "Content-Type": "application/json",
      ...headers,
    },
  });
}
