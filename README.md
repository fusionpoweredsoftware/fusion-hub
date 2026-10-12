# fusion-hub

An AI hub in a single Go binary: a REPL for chatting with language models and running **AIL** apps, and an HTTP API that streams replies as NDJSON. The original JavaScript version is preserved on the `main-legacy` branch.

## Install

```sh
go install github.com/fusionpoweredsoftware/fusion-hub/cmd/fusion-hub@latest
```

## Choosing a model

```text
> set provider as claude          # Anthropic API (default)
> set provider as openai          # or zai, ollama, openrouter, groq, deepseek, xai
> set model as claude-sonnet-5-5  # optional; each provider has a default
> set claude-api-key as sk-...    # or export ANTHROPIC_API_KEY / OPENAI_API_KEY / ...
> set base-url as http://host:11434/v1   # optional override
```

Settings are JSON in `~/.fusion-hub/settings.json` (override the directory with `FUSION_HUB_HOME`). `set` lists them, `set json`, `set json+ {...}` and `set nill <key>` edit them as before.

## The REPL

```text
fusion-hub                       # interactive
fusion-hub start chat-with-ai    # run a command first, then stay interactive
```

`start <command>` makes a command the **sticky command mode**: every line you type goes to it. **Ctrl+C** switches between CLI mode and the command mode (each keeps its own input history); press it twice quickly to exit. Ctrl+C while a reply is streaming cancels it. `help` lists commands.

| Command | What it does |
|---|---|
| `chat-with-ai <message>` | Chat with the AI (no message starts a new chat) |
| `start [new\|old\|last\|undo\|recap\|send\|templated\|new-templated] chat-with-ai [file]` | Chat sessions: `new` archives the current chat under an AI-suggested name, `old <file>` resumes one, `undo`/`last` drops the last exchange, `recap` shows it, `send <file>` sends a file, `new-templated <name>` saves the chat as a template and `templated <name>` starts from one |
| `start [new\|old\|debug] app-using-ai <app> ['{json settings}']` | Run an AIL app on every line you type |
| `multi-command <app> <input>` | Run an AIL app once |
| `say [sync:] [(male-voice)] <text>` | Text to speech (OpenAI voices when an OpenAI key is set, otherwise the system voice) |
| `set`, `help`, `exit` | Settings, help, quit |

Conversations live in `~/.fusion-hub/histories`, templates in `~/.fusion-hub/templates`, actors in `~/.fusion-hub/actors/actors.json`.

## AIL apps

An app is a YAML pipeline. Each step does one thing: `prompt` (with `output: text | json | uidmd`), `print`, `say`, `command`, `set`, `actor`, `foreach`, `repeat`, `save`, `fail` or `stop`. `${...}` templates read variables, step results (by `id`), `input`, `args` and the app's `settings`, with filters `json`, `len`, `upper`, `lower`, `trim` and `default:x`. `when`/`unless` take a template or `a == b`.

```yaml
name: English to Spanish
steps:
  - id: translation
    output: json                       # parsed, retried with the parser error if malformed
    require: [translationToSpanish]    # retried if missing
    ephemeral: true                    # not kept in the chat history
    prompt: 'Translate "${input}" to Spanish. Reply as JSON {"translationToSpanish": "..."}'
  - print: ${translation.translationToSpanish}
```

`output: uidmd` teaches the model UIDMD notation, sends the variables listed in `inputs` as UIDMD blocks and parses the reply's blocks into fields. An `actor` step has an actor answer in character (`id`, or `ids` plus `rounds` for a discussion, `voice` to speak).

Apps are found by path, in `~/.fusion-hub/apps`, or among the built-in examples in [`examples/`](examples): `english2spanish`, `actor`, `re-actor`, `say-more`, `spoken-chat-with-ai`, `text-adventure` and `adventure-design`.

## HTTP API

```sh
fusion-hub serve        # FUSION_HUB_INSTANCE_PORT (3000), FUSION_HUB_INSTANCE_NAME
curl -N -X POST -d '{"message": "tell me about chess"}' localhost:3000/chat-with-ai
curl -N -X POST -d '{"message": "tell me more"}' localhost:3000/chat-with-ai/<sessionId>
```

The reply streams as NDJSON: a `request` line, `message` chunks, then `{"state": "eom"}`, each with `sessionId` and `responseId`. At most `api-request-limit` (2) requests run at once, each slot held for `api-cooldown-ms` (2000) afterwards; extra requests get HTTP 429 with `{"error": 429, "message": "Too many requests."}`. Requests time out after `api-timeout-ms` (120000).

## Development

```sh
go test ./...
```
