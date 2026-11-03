# Security Policy

## Reporting a vulnerability

Please do not open a public issue for anything exploitable (authentication bypass, injection, secret exposure, a node being able to
read or write another node's data, and similar). Use GitHub's private vulnerability reporting: open this repository's **Security** tab
and click **Report a vulnerability**. Include the version, reproduction steps and the impact.

This is an independent fork of [3x-ui](https://github.com/MHSanaei/3x-ui); issues that exist in upstream 3x-ui as well should also be
reported to upstream. Only the latest release receives fixes.

## What the design protects (and what it does not)

See the "Security and limitations" section of the README and `docs/ARCHITECTURE.md` (section 9). In short: nodes authenticate with
certificates issued by the panel's internal CA; data from a node is validated and a node can only affect its own data. The data kept on
a node (cache, journal) is not protected against someone who has root on the node server.
