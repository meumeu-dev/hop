// Cloudflare Worker — hop pairing relay
// Stockage temporaire chiffré, expire après 2 min
// Le worker ne peut JAMAIS lire les données (chiffrées côté client)

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;

    const corsHeaders = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type, X-Pair-Token",
    };

    if (request.method === "OPTIONS") {
      return new Response(null, { headers: corsHeaders });
    }

    // POST /pair — le serveur enregistre ses données chiffrées
    // Retourne un UUID (lookup key) + bearer token (pour auth les actions suivantes)
    if (path === "/pair" && request.method === "POST") {
      const body = await request.json();
      const { data } = body;

      if (!data) {
        return jsonResponse({ error: "missing data" }, 400, corsHeaders);
      }

      // Generate random UUID as lookup key (NOT the code)
      const pairId = crypto.randomUUID();

      // Generate bearer token for this pairing session
      const tokenBytes = new Uint8Array(32);
      crypto.getRandomValues(tokenBytes);
      const token = Array.from(tokenBytes).map(b => b.toString(16).padStart(2, '0')).join('');

      // Hash the token for storage (don't store plaintext)
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

    // GET /pair/:id — le client récupère les données chiffrées (public, mais id is unguessable UUID)
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

    // DELETE /pair/:id — requires bearer token
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

    // POST /pair/:id/response — requires bearer token, only once
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

      // Verify token
      const tokenHash = await sha256(token);
      if (tokenHash !== parsed.tokenHash) {
        return jsonResponse({ error: "unauthorized" }, 401, corsHeaders);
      }

      // Only allow one response
      if (parsed.responsePosted) {
        return jsonResponse({ error: "response already posted" }, 409, corsHeaders);
      }

      const body = await request.json();
      if (!body.data) {
        return jsonResponse({ error: "missing data" }, 400, corsHeaders);
      }

      // Mark as responded
      parsed.responsePosted = true;
      await env.HOP_KV.put(key, JSON.stringify(parsed), { expirationTtl: 120 });

      // Store response
      const responseKey = `${key}:response`;
      await env.HOP_KV.put(responseKey, JSON.stringify({
        data: body.data,
        created: Date.now(),
      }), { expirationTtl: 120 });

      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // GET /pair/:id/response — requires bearer token
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

    // POST /tunnel/:machine_id — register tunnel URL (auth with token)
    if (path.match(/^\/tunnel\/[a-zA-Z0-9_.-]+$/) && request.method === "POST") {
      const machineId = path.split("/tunnel/")[1];
      const body = await request.json();
      const { url: tunnelUrl, token: regToken } = body;

      if (!tunnelUrl || !regToken) {
        return jsonResponse({ error: "missing url or token" }, 400, corsHeaders);
      }

      const key = `tunnel:${machineId}`;
      const tokenHash = await sha256(regToken);

      // Check if already registered with different token
      // Allow override if existing entry is older than 5 minutes (key rotation after reset)
      const existing = await env.HOP_KV.get(key);
      if (existing) {
        const parsed = JSON.parse(existing);
        const age = Date.now() - (parsed.updated || 0);
        if (parsed.tokenHash && parsed.tokenHash !== tokenHash && age < 5 * 60 * 1000) {
          return jsonResponse({ error: "machine already registered with different token" }, 403, corsHeaders);
        }
      }

      await env.HOP_KV.put(key, JSON.stringify({
        url: tunnelUrl,
        tokenHash: tokenHash,
        updated: Date.now(),
      }), { expirationTtl: 3600 }); // 1 hour TTL, machine should re-register

      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // GET /tunnel/:machine_id — resolve tunnel URL (public)
    if (path.match(/^\/tunnel\/[a-zA-Z0-9_.-]+$/) && request.method === "GET") {
      const machineId = path.split("/tunnel/")[1];
      const key = `tunnel:${machineId}`;
      const stored = await env.HOP_KV.get(key);

      if (!stored) {
        return jsonResponse({ error: "not found" }, 404, corsHeaders);
      }

      const parsed = JSON.parse(stored);
      return jsonResponse({ url: parsed.url, updated: parsed.updated }, 200, corsHeaders);
    }

    // Health check
    if (path === "/health") {
      return jsonResponse({ status: "ok", service: "hop-pair" }, 200, corsHeaders);
    }

    return jsonResponse({ error: "not found" }, 404, corsHeaders);
  },
};

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
