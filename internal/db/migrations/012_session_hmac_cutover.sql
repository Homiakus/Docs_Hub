-- P0 session token verification cutover.
--
-- Session tokens were previously stored as SHA256(secret || ':' || token).
-- Runtime now stores HMAC-SHA256(secret, token). Because Docs_Hub is still
-- pre-release, prefer the simplest fail-closed migration: invalidate all
-- pre-cutover sessions once instead of maintaining a dual-read legacy format.
-- Users authenticate again and receive a session using the new MAC format.

DELETE FROM sessions;
