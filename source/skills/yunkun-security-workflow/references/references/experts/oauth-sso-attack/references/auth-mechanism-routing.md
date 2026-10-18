# Auth Mechanism Routing

Use this file before loading deeper auth-federation content.

## Routing Table

| Live signal | Main branch | Next skill |
|---|---|---|
| `redirect_uri`, `client_id`, `state`, `nonce`, `code_challenge`, discovery endpoint | OAuth / OIDC flow | `oauth-jwt-test` |
| `eyJ...`, `Authorization: Bearer`, `kid`, `jku`, `x5u`, `jwks.json` | JWT / token trust | `jwt-attack-methodology` |
| `SAMLRequest`, `SAMLResponse`, `<saml:Assertion>`, ACS endpoint | SAML assertion trust | `saml-sso-assertion-attacks` |
| reset, verify, OTP, MFA, remember-me, local session cookie after SSO | local auth/session edge | `authbypass-test` |

## First Questions

1. Is the weakness in the external identity protocol or the local session binding?
2. Does the app use browser redirects, POSTed assertions, or bearer tokens?
3. What exact value creates the local authenticated state: code exchange, ID token, access token, assertion, or local cookie?
4. Is the attack path cross-user, cross-client, or cross-provider?
