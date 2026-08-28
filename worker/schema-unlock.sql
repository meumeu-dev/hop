-- Config des machines a deverrouiller, synchronisee entre les appareils d'un
-- meme compte hop. `data` est un blob chiffre COTE CLIENT (cle derivee du mot
-- de passe) : le serveur ne peut pas le lire.
CREATE TABLE IF NOT EXISTS unlock_configs (
    account_id TEXT PRIMARY KEY,
    data       TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
