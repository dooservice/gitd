# Stability and deprecation

> **Status: draft, not in effect.** This is what gitd will promise from 1.0.0. The project
> has not declared 1.0.0: until it does, a minor release may still change what is below,
> and says so in its notes. What already holds today is marked as such.

## What will be stable

From 1.0.0, within the same major version:

| Surface | What stays |
|---|---|
| The contract, `proto/gitd/v1` | Services, methods, messages, fields and enum values keep their names, numbers and meaning. New ones may be added. **Holds today:** CI refuses any change `buf breaking` finds against the last release. |
| The command line | Commands, their arguments and flags keep working and meaning the same; `--json` output keeps its fields. New commands, flags and fields may be added. |
| The configuration, `gitd.toml` | Every key keeps its meaning and default. New keys may be added, with defaults that change nothing for a file that does not set them. |
| The Python and TypeScript clients | Public classes, functions and their arguments keep working. New ones may be added. |
| The MCP tools | Tool names and their arguments keep working. No tool will ever take a credential. |
| The state directory | A gitd reads the state of every earlier release of its major version. |

What is not covered: log lines and their wording, metric names beyond `gitd_*` existing,
the layout of files inside the state directory, and anything named experimental in its
documentation.

## How something is deprecated

1. It is marked deprecated in the release that decides it: in the contract with
   `deprecated = true`, in the command line's help and a warning on use, in the clients'
   docstrings, and in the release notes, with what to use instead.
2. It keeps working for at least two minor releases and six months, whichever is longer.
3. It is removed only in the next major release, listed in that release's notes.

A security fix may break this when keeping the old behaviour would leave users exposed;
the release notes say so and why.

## Declaring it

The project declares 1.0.0, and with it this document, when people outside it have run
gitd in production and the contract has held through it. Until then this file stays a
draft.
