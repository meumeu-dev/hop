// Cloudflare Worker — hop pairing relay + accounts
// Stockage temporaire chiffré, expire après 2 min (pairing)
// Comptes: le worker ne peut JAMAIS lire les données machines (chiffrées côté client)

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;

    const corsHeaders = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type, X-Pair-Token, Authorization",
    };

    if (request.method === "OPTIONS") {
      return new Response(null, { headers: corsHeaders });
    }

    // ==================== ACCOUNTS ====================

    // POST /auth/register — create account
    if (path === "/auth/register" && request.method === "POST") {
      const body = await request.json();
      const { email, username, auth_hash } = body;

      if (!email || !username || !auth_hash) {
        return jsonResponse({ error: "missing fields" }, 400, corsHeaders);
      }

      // Validate email format
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
        return jsonResponse({ error: "invalid email" }, 400, corsHeaders);
      }

      // Validate username (alphanumeric, 2-32 chars)
      if (!/^[a-zA-Z0-9_-]{2,32}$/.test(username)) {
        return jsonResponse({ error: "invalid username" }, 400, corsHeaders);
      }

      // Check if email or username already taken
      const existingEmail = await env.HOP_KV.get(`account:email:${email.toLowerCase()}`);
      if (existingEmail) {
        return jsonResponse({ error: "email already registered" }, 409, corsHeaders);
      }

      const existingUser = await env.HOP_KV.get(`account:user:${username.toLowerCase()}`);
      if (existingUser) {
        return jsonResponse({ error: "username already taken" }, 409, corsHeaders);
      }

      // Generate account ID
      const accountId = crypto.randomUUID();

      // Hash the auth_hash again server-side (double hash: client Argon2id -> server SHA256)
      const serverHash = await sha256(auth_hash);

      // Generate session token
      const sessionToken = generateToken();
      const sessionHash = await sha256(sessionToken);

      const account = {
        id: accountId,
        email: email.toLowerCase(),
        username: username.toLowerCase(),
        serverHash: serverHash,
        created: Date.now(),
      };

      // Store account data
      await env.HOP_KV.put(`account:${accountId}`, JSON.stringify(account));
      await env.HOP_KV.put(`account:email:${email.toLowerCase()}`, accountId);
      await env.HOP_KV.put(`account:user:${username.toLowerCase()}`, accountId);

      // Store session (7 days)
      await env.HOP_KV.put(`session:${sessionHash}`, JSON.stringify({
        accountId: accountId,
        created: Date.now(),
      }), { expirationTtl: 7 * 86400 });

      return jsonResponse({
        ok: true,
        account_id: accountId,
        username: username,
        token: sessionToken,
      }, 200, corsHeaders);
    }

    // POST /auth/login — authenticate
    if (path === "/auth/login" && request.method === "POST") {
      const body = await request.json();
      const { email, auth_hash } = body;

      if (!email || !auth_hash) {
        return jsonResponse({ error: "missing fields" }, 400, corsHeaders);
      }

      const accountId = await env.HOP_KV.get(`account:email:${email.toLowerCase()}`);
      if (!accountId) {
        return jsonResponse({ error: "invalid credentials" }, 401, corsHeaders);
      }

      const accountData = await env.HOP_KV.get(`account:${accountId}`);
      if (!accountData) {
        return jsonResponse({ error: "invalid credentials" }, 401, corsHeaders);
      }

      const account = JSON.parse(accountData);
      const serverHash = await sha256(auth_hash);

      if (serverHash !== account.serverHash) {
        return jsonResponse({ error: "invalid credentials" }, 401, corsHeaders);
      }

      // Generate session token
      const sessionToken = generateToken();
      const sessionHash = await sha256(sessionToken);

      await env.HOP_KV.put(`session:${sessionHash}`, JSON.stringify({
        accountId: accountId,
        created: Date.now(),
      }), { expirationTtl: 7 * 86400 });

      return jsonResponse({
        ok: true,
        account_id: accountId,
        username: account.username,
        token: sessionToken,
      }, 200, corsHeaders);
    }

    // GET /account/machines — list machine names (encrypted blobs)
    if (path === "/account/machines" && request.method === "GET") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);

      const machinesData = await env.HOP_KV.get(`machines:${auth.accountId}`);
      const machines = machinesData ? JSON.parse(machinesData) : {};

      return jsonResponse({ ok: true, machines: machines }, 200, corsHeaders);
    }

    // PUT /account/machines — sync all machines (encrypted blob)
    if (path === "/account/machines" && request.method === "PUT") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);

      const body = await request.json();
      if (!body.data) {
        return jsonResponse({ error: "missing data" }, 400, corsHeaders);
      }

      // body.data is an encrypted blob — worker can't read it
      // Store with no expiration (permanent)
      await env.HOP_KV.put(`machines:${auth.accountId}`, JSON.stringify(body.data));

      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // POST /auth/logout — invalidate session
    if (path === "/auth/logout" && request.method === "POST") {
      const token = extractBearerToken(request);
      if (token) {
        const sessionHash = await sha256(token);
        await env.HOP_KV.delete(`session:${sessionHash}`);
      }
      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // ==================== PAIRING ====================

    // POST /pair — le serveur enregistre ses données chiffrées
    if (path === "/pair" && request.method === "POST") {
      const body = await request.json();
      const { data } = body;

      if (!data) {
        return jsonResponse({ error: "missing data" }, 400, corsHeaders);
      }

      const pairId = crypto.randomUUID();
      const tokenBytes = new Uint8Array(32);
      crypto.getRandomValues(tokenBytes);
      const token = Array.from(tokenBytes).map(b => b.toString(16).padStart(2, '0')).join('');
      const tokenHash = await sha256(token);

      const key = `pair:${pairId}`;
      await env.HOP_KV.put(key, JSON.stringify({
        data: data,
        tokenHash: tokenHash,
        created: Date.now(),
        responsePosted: false,
      }), { expirationTtl: 120 });

      return jsonResponse({ ok: true, pair_id: pairId, token: token, expires_in: 120 }, 200, corsHeaders);
    }

    // GET /pair/:id
    if (path.match(/^\/pair\/[0-9a-f-]{36}$/) && request.method === "GET") {
      const pairId = path.split("/pair/")[1];
      const key = `pair:${pairId}`;
      const stored = await env.HOP_KV.get(key);

      if (!stored) {
        return jsonResponse({ error: "not found or expired" }, 404, corsHeaders);
      }

      const parsed = JSON.parse(stored);
      return jsonResponse({ data: parsed.data }, 200, corsHeaders);
    }

    // DELETE /pair/:id
    if (path.match(/^\/pair\/[0-9a-f-]{36}$/) && request.method === "DELETE") {
      const pairId = path.split("/pair/")[1];
      const token = request.headers.get("X-Pair-Token");

      if (!token) {
        return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
      }

      const key = `pair:${pairId}`;
      const stored = await env.HOP_KV.get(key);
      if (!stored) {
        return jsonResponse({ error: "not found" }, 404, corsHeaders);
      }

      const parsed = JSON.parse(stored);
      const tokenHash = await sha256(token);
      if (tokenHash !== parsed.tokenHash) {
        return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
      }

      await env.HOP_KV.delete(key);
      await env.HOP_KV.delete(`${key}:response`);
      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // POST /pair/:id/response
    if (path.match(/^\/pair\/[0-9a-f-]{36}\/response$/) && request.method === "POST") {
      const pairId = path.split("/")[2];
      const token = request.headers.get("X-Pair-Token");

      if (!token) {
        return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
      }

      const key = `pair:${pairId}`;
      const stored = await env.HOP_KV.get(key);
      if (!stored) {
        return jsonResponse({ error: "not found" }, 404, corsHeaders);
      }

      const parsed = JSON.parse(stored);
      const tokenHash = await sha256(token);
      if (tokenHash !== parsed.tokenHash) {
        return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
      }

      if (parsed.responsePosted) {
        return jsonResponse({ error: "response already posted" }, 409, corsHeaders);
      }

      const body = await request.json();
      if (!body.data) {
        return jsonResponse({ error: "missing data" }, 400, corsHeaders);
      }

      parsed.responsePosted = true;
      await env.HOP_KV.put(key, JSON.stringify(parsed), { expirationTtl: 120 });

      const responseKey = `${key}:response`;
      await env.HOP_KV.put(responseKey, JSON.stringify({
        data: body.data,
        created: Date.now(),
      }), { expirationTtl: 120 });

      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // GET /pair/:id/response
    if (path.match(/^\/pair\/[0-9a-f-]{36}\/response$/) && request.method === "GET") {
      const pairId = path.split("/")[2];
      const token = request.headers.get("X-Pair-Token");

      if (!token) {
        return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
      }

      const key = `pair:${pairId}`;
      const stored = await env.HOP_KV.get(key);
      if (!stored) {
        return jsonResponse({ error: "not found" }, 404, corsHeaders);
      }

      const parsed = JSON.parse(stored);
      const tokenHash = await sha256(token);
      if (tokenHash !== parsed.tokenHash) {
        return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
      }

      const responseKey = `${key}:response`;
      const responseStored = await env.HOP_KV.get(responseKey);

      if (!responseStored) {
        return jsonResponse({ error: "no response yet" }, 404, corsHeaders);
      }

      return jsonResponse(JSON.parse(responseStored), 200, corsHeaders);
    }

    // Health check
    if (path === "/health") {
      return jsonResponse({ status: "ok", service: "hop-pair" }, 200, corsHeaders);
    }

    return jsonResponse({ error: "not found" }, 404, corsHeaders);
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
