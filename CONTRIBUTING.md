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
- Use plain descriptions and runnable examples. Avoid slogans and claims the implementation cannot support.
- Keep the [English](README.md), [Japanese](README.ja.md), [Simplified Chinese](README.zh-CN.md), and [German](README.de.md) READMEs aligned when usage or limits change. Use English as the source; preserve commands, flags, file names, and status strings in translations. Have a fluent reviewer check changes to translated prose.

Discuss changes to the two-role game or one-repair rule before implementing them. License selection and contribution terms must be finalized before the public release; no license has been granted by these docs.

See [SECURITY.md](SECURITY.md) for current reporting limits.
