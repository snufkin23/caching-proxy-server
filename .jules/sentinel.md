## 2026-04-23 - SSRF Protection and Header Hardening
**Vulnerability:** Server-Side Request Forgery (SSRF) through target URL parameter and insecure header forwarding.
**Learning:** Blindly forwarding headers in a proxy can leak sensitive credentials (e.g., `Set-Cookie` from upstream). Standard hostname-based SSRF checks are vulnerable to DNS rebinding if resolution and dialing are separate steps.
**Prevention:** Implement a custom `DialContext` to validate IPs at the network level and use an allow-list for both request and response headers.
