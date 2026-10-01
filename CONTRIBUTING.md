# Contributing

Tarigato is an experimental TMLS.NYC project. Start with the [design](docs/design.md).

```sh
go test -race ./...
go vet ./...
go build ./cmd/tarigato
```

Tests use fake agent processes and temporary Go repositories; no model credentials are needed.

- Keep the change small and describe the problem it solves.
- Include a small runnable check for changed behavior.
- State which checks actually ran and which did not. Keep provider qualification separate from controller tests.
- Update documentation and Mermaid diagrams when the workflow changes.
- Write commits and PRs for a public reader: problem, change, validation, and remaining limits. Preserve real authorship and third-party credit.
- Use synthetic examples. Keep credentials, private conversations, customer code, and unrelated local paths out of commits, logs, and screenshots.
- Write English documentation first. Add translations when requested and maintained by a fluent reviewer.

Discuss changes to the two-role game or one-repair rule before implementing them. License selection and contribution terms must be finalized before the public release; no license has been granted by these docs.

See [SECURITY.md](SECURITY.md) for current reporting limits.
