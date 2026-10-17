# Agents' branches on GitHub, without secrets

gitd keeps the token out of every sandbox and refuses a publish that breaks the publishing
rules — a change to `.github/workflows/**`, a deleted test, too many files. What those rules
cannot see is **what the CI runs**: a test, a `Makefile`, the scripts of `package.json`, a
build file. An agent misled by an injected issue can change one of those, and when the CI
runs on its branch, that code runs there, with whatever the CI holds.

GitHub gives repository and organisation secrets to the workflows of every branch of the
same repository — only forks are left out — so a branch gitd publishes receives them unless
the repository says otherwise. The branch prefix is what makes saying otherwise simple: every
agent's branch starts with it.

Set these once per repository (or organisation), with the prefix `agents/`:

## 1. Keep secrets in environments that agents' branches cannot use

Move every secret a job needs beyond reading the code — deploy keys, cloud credentials,
registry tokens, API keys used by tests — from **Repository secrets** into an
**environment**, under *Settings → Environments*:

- **Deployment branches and tags:** *Selected branches and tags*, e.g. `main` and
  `release/*` — never `agents/*`.
- **Required reviewers**, for an environment that deploys.

A job reads an environment's secrets only by referencing the environment, and a job
referencing it from a branch its policy does not allow does not proceed. Leave no secret at
the repository or organisation level that a workflow on any branch could read.

## 2. Make the workflow token read only

*Settings → Actions → General → Workflow permissions:* **Read repository contents and
packages permissions**. A job that needs more asks for it in its own `permissions:` block,
and only on the branches that need it.

## 3. Run nothing privileged for agents' branches

In a workflow that must hold a secret or write, say so on the job:

```yaml
jobs:
  deploy:
    if: ${{ !startsWith(github.ref_name, 'agents/') && !startsWith(github.head_ref, 'agents/') }}
    environment: production
```

Never use `pull_request_target` or `workflow_run` to run code from an agent's branch with
the base repository's permissions.

## 4. Merge only what a person approved

A ruleset on the default branch (*Settings → Rules → Rulesets*): **Require a pull request
before merging**, with at least one approval. A pull request gitd opens comes from the App's
bot, so the person who asked for the work can approve it — and a person does.

## What gitd adds, and what it does not

| Threat | Who stops it |
|---|---|
| An agent reads or sends the git host's token | gitd: the token never reaches a sandbox |
| An agent publishes to `main` or another branch people use | gitd: new branches under the prefix only, never forced |
| An agent changes the CI's workflows, deletes tests, publishes too much | gitd: publishing rules, checked before any push |
| Code an agent changed runs in the CI with secrets | **GitHub**: steps 1–3 above |
| Code an agent changed reaches `main` unreviewed | **GitHub**: step 4 |
