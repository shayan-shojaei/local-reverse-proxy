# Security policy

Please report vulnerabilities privately through GitHub Security Advisories rather than opening a public issue.

The supported release line is the latest tagged minor release. Local Reverse Proxy binds its dashboard, DNS, HTTP, and HTTPS ports only to loopback. Treat the generated installation directory and its `.env` file as credentials: it contains the controller bootstrap token.

The “skip upstream TLS verification” route option deliberately disables authentication of that one upstream and should be used only for a local development server.
