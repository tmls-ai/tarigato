# Security

Tarigato is an experimental TMLS.NYC project; there is no supported release yet.

The tool runs coding agents and generated tests against **trusted local repositories**. Separate workspaces are not sandboxes. Tests and project scripts can access resources available to their operating-system user. Agent permission controls do not automatically protect checks launched by Tarigato.

Use a restricted operating-system account or container when stronger isolation is needed. Local artifacts do not imply offline inference: provider CLIs may transmit task text and source context. Run reports contain the task, patches, and diagnostics; review them before sharing. Unsuccessful runs retain workspaces containing unvalidated changes; do not treat those files as accepted output.

A monitored private reporting route must be configured and verified before the repository is made public. None is currently advertised. Do not post credentials, private source, or sensitive vulnerability details in public issues.

See the [design](docs/design.md) for protected inputs, test handling, and failure states.
