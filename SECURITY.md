# Security

## What Pointbreak can and can't touch
- It reads files in your workspace and writes only inside `<workspace>/context/`.
- It never connects to a database or the network; a test enforces this.
- It never reads secret files named in your rules (`.env*`, `config.yml`, keys). Context files hold names, never values.
- The only command it runs is `git branch --show-current`.
- The pre-write hook only decides whether Claude Code should ask you; it never writes anything itself.

## Reporting a vulnerability
Please don't open a public issue. Use GitHub's private vulnerability reporting on this repository (Security → Report a vulnerability). You'll get a reply within 7 days.
