// Cloudflare Worker — hop pairing relay
// Stockage temporaire chiffré, expire après 5 min
// Le worker ne peut JAMAIS lire les données (chiffrées côté client)

const EXPIRY_MS = 5 * 60 * 1000; // 5 minutes

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const path = url.pathname;

    // CORS headers
    const corsHeaders = {
      "Access-Control-Allow-Origin": "*",
      "Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type",
    };

    if (request.method === "OPTIONS") {
      return new Response(null, { headers: corsHeaders });
    }

    // POST /pair — le rpi enregistre ses données chiffrées
    if (path === "/pair" && request.method === "POST") {
      const body = await request.json();
      const { code, data } = body;

      if (!code || !data) {
        return jsonResponse({ error: "missing code or data" }, 400, corsHeaders);
      }

      // Validate code format (6 digits)
      if (!/^\d{6}$/.test(code)) {
        return jsonResponse({ error: "invalid code format" }, 400, corsHeaders);
      }

      // Store in KV with 5 min TTL
      const key = `pair:${code}`;
      await env.HOP_KV.put(key, JSON.stringify({
        data: data,
        created: Date.now(),
      }), { expirationTtl: 300 }); // 300 seconds = 5 min

      return jsonResponse({ ok: true, expires_in: 300 }, 200, corsHeaders);
    }

    // GET /pair/:code — le PC récupère les données chiffrées
    if (path.startsWith("/pair/") && request.method === "GET") {
      const code = path.split("/pair/")[1];

      if (!/^\d{6}$/.test(code)) {
        return jsonResponse({ error: "invalid code" }, 400, corsHeaders);
      }

      const key = `pair:${code}`;
      const stored = await env.HOP_KV.get(key);

      if (!stored) {
        return jsonResponse({ error: "not found or expired" }, 404, corsHeaders);
      }

      const parsed = JSON.parse(stored);

      // Check expiry (belt and suspenders, KV TTL should handle this)
      if (Date.now() - parsed.created > EXPIRY_MS) {
        await env.HOP_KV.delete(key);
        return jsonResponse({ error: "expired" }, 410, corsHeaders);
      }

      return jsonResponse({ data: parsed.data }, 200, corsHeaders);
    }

    // DELETE /pair/:code — cleanup after successful pairing
    if (path.startsWith("/pair/") && request.method === "DELETE") {
      const code = path.split("/pair/")[1];
      const key = `pair:${code}`;
      await env.HOP_KV.delete(key);
      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // POST /pair/:code/response — le PC envoie sa réponse chiffrée
    if (path.match(/^\/pair\/\d{6}\/response$/) && request.method === "POST") {
      const code = path.split("/")[2];
      const body = await request.json();

      if (!body.data) {
        return jsonResponse({ error: "missing data" }, 400, corsHeaders);
      }

      const key = `pair:${code}:response`;
      await env.HOP_KV.put(key, JSON.stringify({
        data: body.data,
        created: Date.now(),
      }), { expirationTtl: 300 });

      return jsonResponse({ ok: true }, 200, corsHeaders);
    }

    // GET /pair/:code/response — le rpi récupère la réponse du PC
    if (path.match(/^\/pair\/\d{6}\/response$/) && request.method === "GET") {
      const code = path.split("/")[2];
      const key = `pair:${code}:response`;
      const stored = await env.HOP_KV.get(key);

      if (!stored) {
        return jsonResponse({ error: "no response yet" }, 404, corsHeaders);
      }

      return jsonResponse(JSON.parse(stored), 200, corsHeaders);
    }

    // Health check
    if (path === "/health") {
      return jsonResponse({ status: "ok", service: "hop-pair" }, 200, corsHeaders);
    }

    return jsonResponse({ error: "not found" }, 404, corsHeaders);
  },
};

function jsonResponse(data, status, headers = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      "Content-Type": "application/json",
      ...headers,
    },
  });
}
