# Authentication Acceptance

Verified on 2026-07-10 against the local user application.

- Password, email-code, registration, and password-reset presentations render without horizontal overflow.
- Registration sends the backend-supported email-code scene `login`; the email-code login endpoint remains responsible for automatic account creation.
- Delivery cooldown survives mode and intent changes. Reset resend remains disabled while the shared cooldown is active.
- Reset passwords with fewer than eight trimmed characters are rejected locally and focus the new-password field.
- Login tabs use roving focus with ArrowLeft, ArrowRight, Home, and End. Registration and reset do not expose login-mode tab semantics.
- Inline validation is live-announced, reduced-motion transitions resolve to effectively zero duration, and both themes retain visible controls and focus states.

Automated checks:

```text
npm exec --prefix web/user -- tsx web/user/src/pages/loginPresentation.contract.ts
npm exec --prefix web/user -- tsx web/user/src/pages/loginPage.contract.ts
npm --prefix web/user run typecheck
npm --prefix web/user run build
```

Screenshots:

- `screenshots/auth-desktop-light.png`
- `screenshots/auth-desktop-dark.png`
- `screenshots/auth-mobile-code-390.png`
- `screenshots/auth-mobile-reset-320.png`
