# OAuth Redirect And Binding

Use this file for the first-pass flow checks on OAuth and OIDC.

## High-Frequency Weakness Families

| Family | What to test |
|---|---|
| `redirect_uri` validation | suffix or prefix match, `@`, encoded slash, backslash, path confusion, open-redirect chaining |
| `state` handling | missing, static, predictable, or not bound to user session |
| `nonce` handling | missing or not checked on ID token return |
| PKCE | missing for public clients, verifier not enforced |
| local binding | attacker-controlled provider identity bound into victim local account |
| token leakage | code or token in Referer, logs, fragment misuse, callback resources |

## Minimal Flow Map

Capture this sequence once:

1. `/authorize`
2. provider login and consent
3. callback URL
4. local exchange or session creation
5. resulting local account state

## Strong First Tests

- try one `redirect_uri` variant
- remove or replay `state`
- compare one web client and one mobile or SPA client if both exist
- verify whether the callback binds by verified provider identity or by weak email matching
