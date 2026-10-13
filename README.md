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

Settings are JSON in `~/.fusion-hub/fusion-hub/.settings`, the original's location (override the directory with `FUSION_HUB_HOME`). `set` lists them, `set json`, `set json+ {...}` and `set nill <key>` edit them as before.

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
| `multi-command <file> [parameters]` | Run an AIL app once |
| `say [sync:] [(male-voice)] <text>` | Text to speech (OpenAI voices when an OpenAI key is set, otherwise the system voice) |
| `set`, `help`, `exit` | Settings, help, quit |

Conversations live in `~/.fusion-hub/fusion-hub/histories`, templates in `templates`, actors in `actors/actors.json`, apps in `multi-command`.

## AIL apps

An app is a `.app.ail.js` file, exactly as in the original: a header naming the app and the block delimiters, then JavaScript in which every `/*...*/` block is a command. A block without a command word is sent to the AI whole, and `/*(name, type) chat-with-ai ...*/` stores the filtered reply in `name` (`text`, `JSON`, `TrickyJSON`, `JSONNonEnglish` or `UIDMD`, which teaches the model UIDMD notation and sends the listed variables as blocks). The app's `app.input`, `app.settings`, `ai`, `output` and the `fusion-hub/app-lib` modules (`actor`, `otfa-text`, ...) are available as before. The files run unchanged on an embedded JavaScript engine; `start debug app-using-ai` logs the generated module and streams the replies.

```js
//**English to spanish translation app**
//(?:\r\n|\n)[ ]*/\*(?:\r\n|\n)?
//(?:\r?\n?\*/)
//STOP

//script:
/*(response,text)
chat-with-ai ${app.input}*/
output.print(response);
/*say sync: ${response}*/
/*STOP*/
/*start app-using-ai*/
```

Apps live in `~/.fusion-hub/fusion-hub/multi-command` (`start new app-using-ai actor` runs `actor.app.ail.js` there; `./x` is relative to the working directory). The examples in [`example/`](example) and the modules in [`app-lib/`](app-lib) are built into the binary: the first time an example is named it is copied there, so you can read and edit it.

An app can also be a declarative `.ail.yaml` pipeline, named with its extension (`start app-using-ai english2spanish.ail.yaml`, found by path or in `~/.fusion-hub/fusion-hub/apps`). Each step does one thing: `prompt` (with `output: text | json | uidmd`), `print`, `say`, `command`, `set`, `actor`, `foreach`, `repeat`, `save`, `fail` or `stop`; `${...}` templates read variables, step results (by `id`), `input`, `args` and `settings`.

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
