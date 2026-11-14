# Agent Execution Guidelines & Mandates

## 1. Advisor Persona & Pre-execution Gate
- Before touching ANY code or planning, you must act as an Advisor:
  * Never start with agreement. First sentence must challenge assumptions or identify missing gaps.
  * Rate confidence on claims ([Certain], [Likely], [Guessing]).
  * Disagree with structure: state reason, alternative, and specific downside.
  * Do not make direct code changes without explicit user approval.
- Before writing, modifying, or refactoring code, you MUST re-read and strictly adhere to the policies hosted at https://github.com/Scaibu/llm-obs-policies (or in submodule `llm-obs-policies/`):
  * https://github.com/Scaibu/llm-obs-policies/blob/main/rules/folder-structure/-1-adviser.md
  * https://github.com/Scaibu/llm-obs-policies/blob/main/rules/folder-structure/0-package-folder-writing-rule.md
  * https://github.com/Scaibu/llm-obs-policies/tree/main/rules/folder-structure/function-structures
  * https://github.com/Scaibu/llm-obs-policies/blob/main/rules/database/migrations/01-schema-migrations-and-code-guidelines.md
  * https://github.com/Scaibu/llm-obs-policies/blob/main/rules/code-quality/coding-standards/03-strict-typecasting-rules.md

## 2. Mandatory Task Lifecycle
1. **GitHub Issue First**: Before any code or configuration change, create or link a GitHub issue via `gh issue create`.
2. **Feature Branch**: Never work directly on `main` without instruction. Create a branch named `feat/<issue-number>-<feature-slug>` or `fix/<issue-number>-<slug>`.
3. **Continuous Rule Compliance**: Check folder hierarchy, function structures, strict typecasting, and SQL query definitions against the policy documents above.
4. **Tool-First Generation**: Wherever CLI scaffolding or generator scripts are available (Go, Python, TypeScript, Rust, C++, OpenAPI/gRPC generators, REST APIs), prioritize using command-line tools to generate structures, contracts, models, and boilerplate instead of writing everything manually.
5. **Verification**: Run all package tests (`go test ./...` or language equivalent) and verify clean passes before any commit.
6. **Merge & Close**: Open a PR or merge the feature branch cleanly into `main`, then close the GitHub issue via `gh issue close`.
