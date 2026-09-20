// Cloudflare Worker — hop pairing relay + accounts
// Stockage temporaire chiffré, expire après 2 min (pairing)
// Comptes: le worker ne peut JAMAIS lire les données machines (chiffrées côté client)

const MAX_SESSIONS_PER_ACCOUNT = 3;

// ==================== WEB UNLOCK — page boot.meumeu.dev ====================
// Page servie par le worker, protégée par l'app CF Access `hop-boot`
// (email OTP). Elle liste les machines enregistrées dans la KV
// `webunlock:machines`, puis proxy /pubkey et /unlock vers le serveur web
// d'unlock de chaque machine (hostname du tunnel, lui-même derrière une
// policy CF Access service-token only). La passphrase reste chiffrée de bout
// en bout dans le navigateur (RSA-OAEP), le worker ne voit qu'un blob.

const HOP_BOOT_AUD = "80f12c81da7cc64966b4095defe2b7481aa805c9ac5b5b55d0f19f158ecefbd";

const BOOT_PAGE_HTML = `
<!doctype html>
<html lang="fr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>Déverrouillage LUKS</title>
<style>
  :root {
    --bg: #0d1117; --card: #161b22; --border: #30363d;
    --fg: #e6edf3; --muted: #8b949e; --accent: #2f81f7;
    --ok: #3fb950; --err: #f85149;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
    background: radial-gradient(1200px 600px at 50% -10%, #14203a 0%, var(--bg) 60%);
    color: var(--fg); font: 15px/1.5 system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
    padding: 24px;
  }
  .card {
    width: 100%; max-width: 420px; background: var(--card);
    border: 1px solid var(--border); border-radius: 14px; padding: 28px;
    box-shadow: 0 12px 40px rgba(0,0,0,.4);
  }
  .lock { font-size: 34px; text-align: center; }
  h1 { font-size: 18px; margin: 10px 0 2px; text-align: center; font-weight: 600; }
  .sub { text-align: center; color: var(--muted); font-size: 13px; margin-bottom: 18px; }
  .machines { display: flex; flex-direction: column; gap: 8px; margin-bottom: 18px; }
  .machine {
    width: 100%; padding: 12px; text-align: left; font-size: 14px;
    background: #0d1117; color: var(--fg); border: 1px solid var(--border);
    border-radius: 9px; cursor: pointer;
  }
  .machine:hover { border-color: var(--accent); }
  .machine .dot { display: inline-block; width: 9px; height: 9px; border-radius: 50%; margin-right: 8px; }
  .machine .dot.pending { background: var(--ok); }
  .machine .dot.down { background: var(--muted); }
  .machine .meta { float: right; color: var(--muted); font-size: 12px; }
  .detail { display: none; }
  label { display: block; font-size: 13px; color: var(--muted); margin-bottom: 6px; }
  .row { position: relative; }
  input[type=password], input[type=text] {
    width: 100%; padding: 12px 44px 12px 12px; font-size: 15px;
    background: #0d1117; border: 1px solid var(--border); border-radius: 9px;
    color: var(--fg); outline: none; font-family: ui-monospace, monospace;
  }
  input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px rgba(47,129,247,.25); }
  .toggle {
    position: absolute; right: 8px; top: 50%; transform: translateY(-50%);
    background: none; border: 0; color: var(--muted); cursor: pointer; font-size: 13px; padding: 6px;
  }
  button.primary {
    width: 100%; margin-top: 16px; padding: 12px; font-size: 15px; font-weight: 600;
    background: var(--accent); color: #fff; border: 0; border-radius: 9px; cursor: pointer;
  }
  button.primary:disabled { opacity: .55; cursor: not-allowed; }
  .status { margin-top: 16px; font-size: 13px; text-align: center; min-height: 20px; }
  .status.ok { color: var(--ok); }
  .status.err { color: var(--err); }
  .status.info { color: var(--muted); }
  .note { margin-top: 18px; font-size: 11.5px; color: var(--muted); text-align: center; line-height: 1.5; }
  .spin { display: inline-block; width: 13px; height: 13px; border: 2px solid var(--muted);
    border-top-color: transparent; border-radius: 50%; animation: r .7s linear infinite; vertical-align: -2px; margin-right: 6px; }
  @keyframes r { to { transform: rotate(360deg); } }
</style>
</head>
<body>
  <div class="card">
    <div class="lock">🔒</div>
    <h1>Déverrouillage LUKS</h1>
    <div class="sub" id="sub">Choisis une machine.</div>
    <div class="machines" id="machines"></div>
    <div class="detail" id="detail">
      <label for="pass">Passphrase LUKS</label>
      <div class="row">
        <input id="pass" type="password" autocomplete="off" autofocus spellcheck="false" enterkeyhint="go">
        <button type="button" class="toggle" id="toggle" aria-label="Afficher">voir</button>
      </div>
      <button type="button" class="primary" id="submit">Déverrouiller</button>
      <div class="status info" id="status">Chargement…</div>
    </div>
    <div class="note">
      La passphrase est chiffrée dans ton navigateur (RSA-OAEP) avec une clé
      éphémère de la machine. Cloudflare ne voit qu'un blob chiffré.
    </div>
  </div>
<script>
(function () {
  "use strict";
  var pubKey = null;
  var current = null;
  var machinesEl = document.getElementById("machines");
  var sub = document.getElementById("sub");
  var detail = document.getElementById("detail");
  var pass = document.getElementById("pass");
  var submit = document.getElementById("submit");
  var status = document.getElementById("status");
  var toggle = document.getElementById("toggle");

  function setStatus(msg, kind, spin) {
    status.className = "status " + (kind || "info");
    status.innerHTML = (spin ? '<span class="spin"></span>' : "") + msg;
  }
  function b64ToBuf(b64) {
    var bin = atob(b64), buf = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i);
    return buf;
  }
  function bufToB64(buf) {
    var bin = "", bytes = new Uint8Array(buf);
    for (var i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
    return btoa(bin);
  }
  toggle.addEventListener("click", function () {
    var show = pass.type === "password";
    pass.type = show ? "text" : "password";
    toggle.textContent = show ? "masquer" : "voir";
    pass.focus();
  });

  function refresh() {
    return fetch("machines", { cache: "no-store" })
      .then(function (r) { if (!r.ok) throw new Error("status " + r.status); return r.json(); })
      .then(function (j) {
        if (!j.ok) throw new Error("resp");
        machinesEl.innerHTML = "";
        (j.machines || []).forEach(function (m) {
          var b = document.createElement("button");
          b.className = "machine";
          b.type = "button";
          var dot = document.createElement("span");
          dot.className = "dot " + (m.status === "down" ? "down" : "pending");
          b.appendChild(dot);
          b.appendChild(document.createTextNode(m.machine_id + " " + (m.hostname || "")));
          var meta = document.createElement("span");
          meta.className = "meta";
          meta.textContent = m.status === "down" ? "offline" : (m.since ? "en boot" : "—");
          b.appendChild(meta);
          b.addEventListener("click", function () { select(m); });
          machinesEl.appendChild(b);
        });
        sub.textContent = "Choisis une machine.";
      })
      .catch(function (e) {
        setStatus("Machines indisponibles : " + e.message, "err");
        setTimeout(refresh, 5000);
      });
  }

  function select(m) {
    current = m;
    detail.style.display = "block";
    sub.textContent = "Machine : " + m.machine_id;
    machinesEl.querySelectorAll(".machine").forEach(function (el) { el.style.borderColor = ""; });
    loadKey();
  }

  function loadKey() {
    pubKey = null;
    setStatus("Préparation…", "info", true);
    submit.disabled = true;
    fetch("pubkey?machine=" + encodeURIComponent(current.machine_id), { cache: "no-store" })
      .then(function (r) { if (!r.ok) throw new Error("pubkey " + r.status); return r.json(); })
      .then(function (j) {
        return crypto.subtle.importKey(
          "spki", b64ToBuf(j.pubkey),
          { name: "RSA-OAEP", hash: "SHA-256" }, false, ["encrypt"]);
      })
      .then(function (k) {
        pubKey = k;
        submit.disabled = false;
        setStatus("Prêt.", "info");
      })
      .catch(function (e) {
        setStatus("Impossible de contacter la machine. (" + e.message + ")", "err");
      });
  }

  submit.addEventListener("click", function () {
    if (!current) return;
    if (!pubKey) { loadKey(); return; }
    var value = pass.value;
    if (!value) { setStatus("Saisis la passphrase.", "err"); pass.focus(); return; }
    submit.disabled = true;
    setStatus("Déverrouillage en cours…", "info", true);
    var data = new TextEncoder().encode(value);
    crypto.subtle.encrypt({ name: "RSA-OAEP" }, pubKey, data)
      .then(function (ct) {
        return fetch("unlock?machine=" + encodeURIComponent(current.machine_id), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ blob: bufToB64(ct) })
        });
      })
      .then(function (r) { return r.json().then(function (j) { return { r: r, j: j }; }); })
      .then(function (res) {
        if (res.j.ok) {
          setStatus("✓ " + (res.j.msg || "Disque déverrouillé, la machine démarre."), "ok");
          pass.value = ""; pass.disabled = true; toggle.disabled = true;
        } else {
          setStatus("✗ " + (res.j.error || "Échec."), "err");
          submit.disabled = false;
          pass.select();
          if (res.r.status === 400) loadKey();
        }
      })
      .catch(function (e) {
        setStatus("Erreur réseau : " + e.message, "err");
        submit.disabled = false;
      });
  });

  if (!window.crypto || !crypto.subtle) {
    sub.textContent = "Pas de WebCrypto (HTTPS requis).";
  } else {
    refresh();
  }
})();
</script>
</body>
</html>
`;

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

    // GET /account/unlock — blob chiffre cote client des machines a
    // deverrouiller. Endpoint distinct de /account/machines pour ne pas
    // casser le format attendu par le CLI hop. Le Worker ne peut pas le lire
    // (chiffre avec la cle derivee du mot de passe, jamais transmise).
    if (path === "/account/unlock" && request.method === "GET") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, cors);

      const row = await env.HOP_UNLOCK_DB
        .prepare("SELECT data FROM unlock_configs WHERE account_id = ?")
        .bind(auth.accountId)
        .first();
      return jsonResponse({ ok: true, data: row?.data || "" }, 200, cors);
    }

    // PUT /account/unlock
    if (path === "/account/unlock" && request.method === "PUT") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, cors);

      const contentLength = parseInt(request.headers.get("Content-Length") || "0");
      if (contentLength > 1048576) {
        return jsonResponse({ error: "payload too large" }, 413, cors);
      }

      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      if (typeof body.data !== "string" || body.data.length > 1048576) {
        return jsonResponse({ error: "invalid data" }, 400, cors);
      }

      await env.HOP_UNLOCK_DB
        .prepare("INSERT OR REPLACE INTO unlock_configs (account_id, data, updated_at) VALUES (?, ?, ?)")
        .bind(auth.accountId, body.data, Date.now())
        .run();
      return jsonResponse({ ok: true }, 200, cors);
    }

    // DELETE /account/unlock — retire la config synchronisee du cloud
    if (path === "/account/unlock" && request.method === "DELETE") {
      const auth = await authenticateRequest(request, env);
      if (!auth) return jsonResponse({ error: "unauthorized" }, 401, cors);

      await env.HOP_UNLOCK_DB
        .prepare("DELETE FROM unlock_configs WHERE account_id = ?")
        .bind(auth.accountId)
        .run();
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
      await env.HOP_UNLOCK_DB
        .prepare("DELETE FROM unlock_configs WHERE account_id = ?")
        .bind(auth.accountId)
        .run();

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

    // ==================== UNLOCK (push-approval LUKS unlock) ====================

    // POST /unlock/trigger — appelé par l'initramfs d'une machine LUKS au boot
    // Body: { machine_id, nonce, timestamp, signature }
    // signature = HMAC-SHA256(secret_machine, `${machine_id}:${nonce}:${timestamp}`)
    if (path === "/unlock/trigger" && request.method === "POST") {
      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      const { machine_id, nonce, timestamp, signature } = body;
      if (!machine_id || !nonce || !timestamp || !signature) {
        return jsonResponse({ error: "missing fields" }, 400, cors);
      }

      // Validation stricte des formats AVANT tout usage : machine_id sert
      // d'index dans env[] et de composant de cle KV, nonce de cle KV.
      if (!isValidMachineId(machine_id) || !isValidNonce(nonce) ||
          typeof timestamp !== "number" || !Number.isFinite(timestamp)) {
        return jsonResponse({ error: "invalid fields" }, 400, cors);
      }

      // Rate limit par IP (et pas par machine_id) AVANT verification : sinon
      // n'importe qui peut envoyer 10 requetes bidons avec machine_id=megahost
      // et bloquer le vrai trigger pendant 15 min.
      const trigIp = request.headers.get("CF-Connecting-IP") || "unknown";
      const rlKey = `ratelimit:unlock-trigger:${trigIp}`;
      const rlCount = parseInt(await env.HOP_KV.get(rlKey) || "0");
      if (rlCount >= 30) return jsonResponse({ error: "rate limit" }, 429, cors);
      await env.HOP_KV.put(rlKey, String(rlCount + 1), { expirationTtl: 900 });

      // Fraicheur du timestamp (anti-replay, fenetre 2 min)
      const now = Math.floor(Date.now() / 1000);
      if (Math.abs(now - timestamp) > 120) {
        return jsonResponse({ error: "stale timestamp" }, 400, cors);
      }

      // Reponse identique si la machine est inconnue ou la signature fausse :
      // evite de transformer l'endpoint en oracle revelant quelles machines
      // (donc quels secrets env) existent.
      const secret = env[`UNLOCK_HMAC_SECRET_${machine_id.toUpperCase()}`];
      const valid = secret
        ? await verifyHmac(secret, `${machine_id}:${nonce}:${timestamp}`, signature)
        : false;
      if (!valid) return jsonResponse({ error: "unauthorized" }, 401, cors);

      // Nonce a usage unique — verifie APRES la signature, pour qu'un tiers ne
      // puisse pas polluer l'espace de cles KV sans connaitre le secret.
      const nonceKey = `unlock:nonce:${machine_id}:${nonce}`;
      if (await env.HOP_KV.get(nonceKey)) {
        return jsonResponse({ error: "replayed nonce" }, 400, cors);
      }
      await env.HOP_KV.put(nonceKey, "1", { expirationTtl: 300 });

      // TTL 24h — megahost peut rester en attente longtemps.
      const pending = {
        machine_id,
        nonce,
        status: "pending",
        created: now,
      };
      await env.HOP_KV.put(`unlock:pending:${machine_id}`, JSON.stringify(pending), { expirationTtl: 86400 });

      // La notification ntfy est envoyee directement par l'initramfs de la
      // machine (quota ntfy.sh par IP source : les IP partagees des Workers
      // Cloudflare sont deja saturees, HTTP 429 systematique).

      return jsonResponse({ ok: true }, 200, cors);
    }

    // GET /unlock/status?machine_id= — l'app demande si une machine attend un
    // unlock (authentifie par la session de compte hop existante). Pas de
    // push : c'est l'app qui interroge quand l'utilisateur l'ouvre.
    if (path === "/unlock/status" && request.method === "GET") {
      const account = await authenticateRequest(request, env);
      if (!account) return jsonResponse({ error: "unauthorized" }, 401, cors);

      const machineId = url.searchParams.get("machine_id");
      if (!isValidMachineId(machineId)) return jsonResponse({ error: "invalid machine_id" }, 400, cors);

      const pendingRaw = await env.HOP_KV.get(`unlock:pending:${machineId}`);
      if (!pendingRaw) return jsonResponse({ pending: false }, 200, cors);
      const pending = JSON.parse(pendingRaw);
      return jsonResponse({ pending: true, since: pending.created }, 200, cors);
    }

    // POST /unlock/clear — l'app signale que la machine a ete deverrouillee
    // (efface l'etat d'attente). Purement informatif, aucun effet securite.
    if (path === "/unlock/clear" && request.method === "POST") {
      const account = await authenticateRequest(request, env);
      if (!account) return jsonResponse({ error: "unauthorized" }, 401, cors);

      let body;
      try { body = await request.json(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      const { machine_id } = body;
      if (!isValidMachineId(machine_id)) return jsonResponse({ error: "invalid machine_id" }, 400, cors);

      await env.HOP_KV.delete(`unlock:pending:${machine_id}`);
      return jsonResponse({ ok: true }, 200, cors);
    }

    // ==================== WEB UNLOCK (boot.meumeu.dev) ====================
    // La page et les API sont exposées uniquement derrière l'app CF Access
    // `hop-boot` (email OTP) sur boot.meumeu.dev. Les endpoints sensibles
    // (proxy /pubkey + /unlock) vérifient en plus le JWT d'Access pour ne
    // jamais accepter une requête venant du hostname workers.dev (non
    // protégé par Access), qui serait sinon un proxy ouvert vers les machines.

    // GET /machines — liste des machines web-unlock connues + état d'attente
    if (path === "/machines" && request.method === "GET") {
      if (!await verifyAccessJwt(request, env)) {
        return jsonResponse({ error: "unauthorized" }, 401, cors);
      }
      const raw = await env.HOP_KV.get("webunlock:machines");
      const registry = raw ? JSON.parse(raw) : {};
      const machineIds = Object.keys(registry).filter(isValidMachineId);
      const out = [];
      for (const mid of machineIds) {
        const pendingRaw = await env.HOP_KV.get(`unlock:pending:${mid}`);
        const pending = pendingRaw ? JSON.parse(pendingRaw) : null;
        out.push({
          machine_id: mid,
          hostname: registry[mid].hostname || "",
          status: pending && pending.status === "pending" ? "pending" : "down",
          since: pending ? pending.created : null,
        });
      }
      return jsonResponse({ ok: true, machines: out }, 200, cors);
    }

    // GET /pubkey?machine=X — proxy vers la clé éphémère du serveur web de la
    // machine (identifiée par le service token). La réponse est retransmise
    // telle quelle; on ne limite que la taille (2048-bit => ~450 bytes).
    if (path === "/pubkey" && request.method === "GET") {
      if (!await verifyAccessJwt(request, env)) {
        return jsonResponse({ error: "unauthorized" }, 401, cors);
      }
      const machine = url.searchParams.get("machine");
      if (!isValidMachineId(machine)) return jsonResponse({ error: "invalid machine" }, 400, cors);
      const backend = await machineBackend(env, machine);
      if (!backend) return jsonResponse({ error: "unknown machine" }, 404, cors);
      const upstream = await fetch(`https://${backend.hostname}/pubkey`, {
        headers: accessTokenHeaders(env),
      });
      const body = await upstream.text();
      return new Response(body, {
        status: upstream.status,
        headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
      });
    }

    // POST /unlock?machine=X — proxy du blob chiffré vers la machine
    if (path === "/unlock" && request.method === "POST") {
      if (!await verifyAccessJwt(request, env)) {
        return jsonResponse({ error: "unauthorized" }, 401, cors);
      }
      const machine = url.searchParams.get("machine");
      if (!isValidMachineId(machine)) return jsonResponse({ error: "invalid machine" }, 400, cors);
      const backend = await machineBackend(env, machine);
      if (!backend) return jsonResponse({ error: "unknown machine" }, 404, cors);

      let body;
      try { body = await request.text(); } catch { return jsonResponse({ error: "bad request" }, 400, cors); }
      if (!body || body.length > 16 << 10) return jsonResponse({ error: "bad request" }, 400, cors);

      const upstream = await fetch(`https://${backend.hostname}/unlock`, {
        method: "POST",
        headers: {
          ...accessTokenHeaders(env),
          "Content-Type": "application/json",
        },
        body: body,
      });
      const text = await upstream.text();
      return new Response(text, {
        status: upstream.status,
        headers: { "Content-Type": "application/json", "Cache-Control": "no-store" },
      });
    }

    // GET /boot — la page web de déverrouillage (ne nécessite pas de proxy :
    // la page elle-même est servie derrière l'app Access hop-boot).
    if (path === "/boot" && request.method === "GET") {
      return new Response(BOOT_PAGE_HTML, {
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          "Cache-Control": "no-store",
          "Content-Security-Policy":
            "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'",
          "X-Content-Type-Options": "nosniff",
          "Referrer-Policy": "no-referrer",
        },
      });
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

// ==================== UNLOCK HELPERS ====================

// machine_id sert d'index dans env[] et de composant de cle KV : on le borne
// strictement pour eviter toute injection de cle ou sondage de variables.
function isValidMachineId(s) {
  return typeof s === "string" && /^[a-zA-Z0-9_-]{1,32}$/.test(s);
}

function isValidNonce(s) {
  return typeof s === "string" && /^[a-zA-Z0-9_-]{1,64}$/.test(s);
}

// Service token CF Access partagé « hop-boot-worker » : le worker s'identifie
// auprès des serveurs web d'unlock des machines (policy non_identity +
// service_token). Env: BOOT_SERVICE_TOKEN_ID / BOOT_SERVICE_TOKEN_SECRET.
function accessTokenHeaders(env) {
  return {
    "Cf-Access-Client-Id": env.BOOT_SERVICE_TOKEN_ID || "",
    "Cf-Access-Client-Secret": env.BOOT_SERVICE_TOKEN_SECRET || "",
  };
}

// Résout le backend web d'une machine depuis la registry KV webunlock:machines
// = { "<machine_id>": { "hostname": "unlock-web-x.meumeu.dev" } }.
async function machineBackend(env, machineId) {
  const raw = await env.HOP_KV.get("webunlock:machines");
  if (!raw) return null;
  let registry;
  try { registry = JSON.parse(raw); } catch { return null; }
  const entry = registry[machineId];
  if (!entry || typeof entry.hostname !== "string") return null;
  if (!/^[a-zA-Z0-9][a-zA-Z0-9.-]{1,253}$/.test(entry.hostname)) return null;
  return entry;
}

// JWKS du team Access (meumeu-dev.cloudflareaccess.com), mis en cache en KV 1h
// https://developers.cloudflare.com/cloudflare-one/identity/authorization-cookie/validating-json/
async function getAccessJwks(env) {
  const cacheKey = "webunlock:jwks";
  const cached = await env.HOP_KV.get(cacheKey);
  if (cached) return JSON.parse(cached);
  const res = await fetch("https://meumeu-dev.cloudflareaccess.com/cdn-cgi/access/certs");
  if (!res.ok) return null;
  const jwks = await res.json();
  await env.HOP_KV.put(cacheKey, JSON.stringify(jwks), { expirationTtl: 3600 });
  return jwks;
}

function base64UrlDecode(str) {
  const b64 = str.replace(/-/g, "+").replace(/_/g, "/").padEnd(str.length + ((4 - (str.length % 4)) % 4), "=");
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

async function verifyAccessJwt(request, env) {
  try {
    const token = request.headers.get("Cf-Access-Jwt-Assertion");
    if (!token) return false;
    const parts = token.split(".");
    if (parts.length !== 3) return false;
    const header = JSON.parse(new TextDecoder().decode(base64UrlDecode(parts[0])));
    const payload = JSON.parse(new TextDecoder().decode(base64UrlDecode(parts[1])));

    // aud de l'app hop-boot (boot.meumeu.dev) + date d'expiration
    if (payload.aud !== HOP_BOOT_AUD) return false;
    if (payload.exp && payload.exp < Math.floor(Date.now() / 1000)) return false;

    const jwks = await getAccessJwks(env);
    if (!jwks) return false;
    const key = (jwks.keys || []).find(k => k.kid === header.kid);
    if (!key) return false;

    const publicKey = await crypto.subtle.importKey(
      "jwk", { kty: key.kty, n: key.n, e: key.e }, { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }, false, ["verify"]);
    const signature = base64UrlDecode(parts[2]);
    const data = new TextEncoder().encode(parts[0] + "." + parts[1]);
    return await crypto.subtle.verify("RSASSA-PKCS1-v1_5", publicKey, signature, data);
  } catch {
    return false;
  }
}

function hexToBytes(hex) {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = parseInt(hex.substr(i * 2, 2), 16);
  }
  return bytes;
}

async function verifyHmac(secretHex, message, signatureHex) {
  try {
    const key = await crypto.subtle.importKey(
      "raw", hexToBytes(secretHex), { name: "HMAC", hash: "SHA-256" }, false, ["verify"]
    );
    return await crypto.subtle.verify("HMAC", key, hexToBytes(signatureHex), new TextEncoder().encode(message));
  } catch {
    return false;
  }
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
