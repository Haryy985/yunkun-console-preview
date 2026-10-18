---
name: oauth-sso-attack
description: OAuth 2.0 / OIDC / SSO authentication-flow router and attack guide. Use when the target shows third-party login, redirect_uri parameters, authorization-code flows, SSO callbacks, state or nonce handling, PKCE, or discovery endpoints. Cover redirect_uri hijack, state-missing CSRF, token leakage, scope abuse, and route JWT, SAML, and auth-misc issues into narrower specialist skills.
metadata:
  tags: "oauth,sso,openid,oidc,redirect_uri,authorization,token,login,session,mfa,cas,saml"
  category: "exploit"
---

# OAuth SSO Attack Router

Use this skill as the **mechanism router** for auth-federation surfaces.

## Read Order

- Read `references/auth-mechanism-routing.md` on every use.
- If the live issue is OAuth or OIDC flow validation, read `references/oauth-redirect-and-binding.md` and then route into `oauth-jwt-test`.
- If the live issue is a JWT or bearer token, route into `jwt-attack-methodology`.
- If the live issue is a SAML response or assertion, route into `saml-sso-assertion-attacks`.
- If the live issue is reset, 2FA, OTP, remember-me, or local session state after SSO, route into `authbypass-test`.

## First Goal

Do not start with payload dumping. First answer:

1. which auth mechanism is live
2. where the callback or assertion lands
3. which artifact creates the local session
4. whether the weakness is in flow validation, token trust, assertion trust, or local binding

## High-Value Outcomes

- redirect_uri or callback abuse -> code or token sent to attacker
- state or nonce weakness -> login CSRF or forced account binding
- token trust weakness -> forged or replayed identity
- SAML trust weakness -> arbitrary NameID or assertion acceptance
- local binding flaw -> victim session linked to attacker-controlled external identity
