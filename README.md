# Fusion Hub

Fusion Hub is a modular Node.js **command-line hub** and **streaming HTTP API** for chatting with Google's **Gemini** model through **Vertex AI**. It also includes **Scenario**, an AI-judged problem-solving party game. Every command is a plug-in module in `command/`, and the same commands can be run interactively, from scripts, or over HTTP.

## Contents

1. [Requirements](#requirements)
2. [Installation](#installation)
3. [Running](#running)
4. [The CLI](#the-cli)
5. [Commands](#commands)
6. [Scenario game](#scenario-game)
7. [HTTP API](#http-api)
8. [Configuration](#configuration)
9. [Files and storage](#files-and-storage)
10. [Project layout](#project-layout)
11. [Programmatic use](#programmatic-use)
12. [Writing a new command](#writing-a-new-command)
13. [Known quirks](#known-quirks)

## Requirements

- **Node.js 16+**. The code uses ES modules, top-level `await` and `||=`.
- A **Google Cloud project** with the Vertex AI API enabled.
- **Application Default Credentials** on the machine:
  ```sh
  gcloud auth application-default login
  ```
- The Vertex project and region are hardcoded in `ai/api/vertex.mjs` (`project: 'utility-time-388516'` and `location: 'us-central1'`). Change them to your own project.

## Installation

```sh
git clone https://github.com/fusionpoweredsoftware/fusion-hub.git
cd fusion-hub
npm install
npm install express        # needed only for the HTTP API (not declared in package.json)
cd testing && npm install  # links fusion-hub into testing/ as a dependency
```

The package refers to itself as `fusion-hub/...` (through `exports` in `package.json`), so code should be run from a package that depends on it. `testing/` is set up that way.

## Running

All of the runners live in `testing/`:

| Script | What it does |
|---|---|
| `node console-test.mjs [initial command]` | Starts the interactive CLI. Any arguments are run as the first command, for example `node console-test.mjs start scenario`. |
| `node console-test-with-initial-input.mjs` | Runs a single `chat-with-ai` command non-interactively. |
| `node api-test.mjs` | Starts a fresh AI chat, then starts the HTTP API on port 3000. |
| `sh sendChat.sh "message"` | Sends a new chat request to the API. |
| `sh sendChatFollowup.sh <sessionId> "message"` | Sends a follow-up message within an existing session. |

## The CLI

- The prompt is `> `. Type a **command name** followed by its arguments, for example `chat-with-ai Hello!`.
- **Command mode.** `start <command>` locks the prompt to that command (the prompt becomes `chat-with-ai> `, for example). After that, every line you type is sent straight to the command, with no prefix.
- **Ctrl+C / Ctrl+D** works like this:
  - In command mode, it returns you to the main prompt.
  - At the main prompt, it re-enters the last command mode.
  - Pressing it **twice within 350 ms** exits.
- **History.** The main prompt and command mode each keep their own ↑/↓ history, and both persist between runs.
- Log lines are timestamped as `[YYYY-MM-DD HH:MM:SS.mmm] - message`.
- Unless a command is marked case-sensitive, its input is lowercased.

## Commands

| Command | Description | Example |
|---|---|---|
| `help` | Lists every command with its description and examples. | `help` |
| `start` | Starts a session of a *startable* command and switches into its command mode. | `start new chat-with-ai` |
| `chat-with-ai` | Sends a message to the AI and streams back the reply. With no message, it starts a new chat. | `chat-with-ai Hello, Mr. AI!` |
| `scenario` | The Scenario game (see [below](#scenario-game)). | `start scenario` |
| `exit` | Exits the app. | `exit` |

### `start`

```
start <command> [args...]
start <type> <command> [args...]
```

Startable commands are **`chat-with-ai`** and **`scenario`**.

The **start types for `chat-with-ai`** are:

| Type | Effect |
|---|---|
| *(none)* | Resumes the current conversation if one exists. Otherwise it starts a new one. |
| `new` | Archives the current conversation and starts fresh. The AI names the archive (for example `chess-strategy.json`); if that name exists, ` (2)`, ` (3)` and so on are added, and if it can't name it, a timestamp is used. |
| `old <file>` | Loads a saved conversation. A bare filename is looked up in the histories folder. Paths starting with `.`, `/` or `~` are used as-is. Before loading, it writes `.bak` and rotating `.old0`–`.old9` backups. |
| `last` | **Undoes** the last exchange and shows the one before it. |
| `recap` | Re-displays the last question and answer. |
| `send <file>` | Sends the contents of a file to the AI as one message. |
| `template <file>` | Copies a history into the `templates` folder and starts from it, without changing the original. |

The **start types for `scenario`** are `new` (which resets everything) or none. Either can be followed by comma-separated themes, as described below.

### `chat-with-ai` details

- Every conversation starts with a hidden instruction: *"Only speak in English unless instructed otherwise."* (see `ai/instructions.mjs`).
- Model settings:
  - model `gemini-pro`
  - `maxOutputTokens` 2048
  - `temperature` 0.9
  - `topP` 1
  - dangerous content is blocked only at the **high** threshold
- Replies **stream** to the terminal, and history is saved after every message.
- Empty model replies are cleaned from history automatically. If a reply comes back empty and the message has a `!` stuck to a word, it retries once with a space before the `!`.
- A `429 Too Many Requests` error is shown as *"The AI is currently busy. Try again later."*

## Scenario game

Scenario is a **two-player**, hot-seat item puzzle. The AI invents a short emergency, each player secretly commits **3 items** from their hand, and the AI decides which 3 items from everything played best solve it.

### Starting

```
start scenario                         # random theme each round
start scenario Bank-Heist,Art-Theft    # restrict to these themes
start new scenario                     # full reset (scores, hands, rounds)
```

The CLI enters `scenario>` mode, so from then on you only type the moves below.

### Themes

These are picked at random each round unless you restrict them:

Mystery-in-a-Mansion, Surviving-a-Deserted-Island, Cybersecurity-Breach, Medical-Outbreak, Time-Travel-Conundrum, Space-Exploration, Post-Apocalyptic-Survival, Environmental-Crisis, Political-Intrigue, Bank-Heist, Art-Theft, Haunted-House, Apartment-Escape, Dystopian-Society, Lost-in-a-Labyrinth, Alien-Invasion, Undercover-Spy, Pirate-Treasure-Hunt, Cyberpunk-City, Olympic-Challenge.

The AI is asked for a realistic, unique, **one-sentence** problem that needs a quick solution.

### Items

Each player holds a **hand of 9 items** drawn from a pool of about **100**. The pool covers:

- protective gear (helmet, goggles, respirator…)
- first aid (tourniquet, splints, defibrillator…)
- tools (crowbar, multi-tool, duct tape…)
- outdoor and survival gear (compass, water filter, signaling device…)
- household and garden items (garden hose, wheelbarrow, lawnmower…)
- odds and ends (telescope, lockpick set, sewing kit, binoculars, portable radio…)

The full list is in `command/scenario.mjs`. **Unused items carry over** to the next round, and your hand is refilled with new, non-duplicate items up to 9.

### Moves (in `scenario>` mode)

| Input | Action |
|---|---|
| `1 4 7` | **Play** the items numbered 1, 4 and 7. |
| `d 2 5 8` / `drop 2 5 8` | **Discard** up to 3 items. This is only allowed while you hold more than 6 items and the round isn't over. The items are replaced at the next refill. |
| `a [0-7]` / `ai [0-7]` | Let the **AI pick for you**. The optional number is a difficulty (0–7) that adds that many extra items for the AI to consider before your hand is trimmed back to 9. |
| `n [themes]` / `next [themes]` | Starts the **next round**, optionally with new themes. |

### Round flow

1. A round number and scenario are shown, along with **Player 1's** hand.
2. Player 1 plays 3 items. Their remaining hand is shown, and then it's **Player 2's** turn with the same scenario.
3. After both players have played, a **recap** lists everyone's items, and the AI is shown all 6 played items. It picks the best 3 and explains why, with the chosen items in **bold**.
4. Each player scores based on how many of their items the AI picked:

   | AI picks from your items | Points |
   |---|---|
   | 0 | 0 |
   | 1 | 1 |
   | 2 | 3 |
   | 3 | 6 |

5. **First-round handicap.** A player who scores 0 in round 1 gets an **extra item** (a hand of 10).
6. *"Round has ended."* Type `n` to continue.

The AI's answer is validated: it must return exactly 3 items, spelled as listed, from the allowed items only. The game re-prompts it up to 5 times. If it still fails, the round is tossed with no points awarded.

## HTTP API

Start the server with `node testing/api-test.mjs`. It listens on `FUSION_HUB_INSTANCE_PORT` (default **3000**).

### `POST /chat-with-ai/:sessionId?`

The request body is:

```json
{ "message": "tell me about chess" }
```

- If you leave out `sessionId`, a new one is generated and returned. Pass it back on later calls to continue the same conversation.
- Each session's history is stored in its own file (`.ai-chat-history.json.<sessionId>`).

The response is **chunked, newline-delimited JSON**:

```
{"request":"tell me about chess","responseId":"…","sessionId":"…"}
{"message":"Chess is ","responseId":"…","sessionId":"…"}
{"message":"a two-player…","responseId":"…","sessionId":"…"}
{"state":"eom","responseId":"…","sessionId":"…"}
```

`state: "eom"` marks the end of the message. Use `curl -N` to see the stream live.

Throttling:

- Up to `asyncRequestLimit` (2) requests can run at once; beyond that you get `{"error":429,"message":"Too many requests."}`.
- A request slot is freed `waitBetweenRequestsMS` (2 s) after its response finishes, or after `asyncRequestTimeoutMS` (2 min) at most.

Routes are defined in `api/default-config.mjs`. Each route has the following hooks:

| Hook | Role |
|---|---|
| `preRequest` | Runs once at startup. |
| `request` | Maps the body to a list of `executeCommand` actions. |
| `preResponse`, `response`, `postResponse` | Format the header chunk, the streamed chunks and the closing chunk. |

## Configuration

| Setting | Where | Default |
|---|---|---|
| `FUSION_HUB_INSTANCE_NAME` | env | `Fusion Hub` |
| `FUSION_HUB_INSTANCE_PORT` | env | `3000` |
| Vertex project and region | `ai/api/vertex.mjs` | `utility-time-388516`, `us-central1` |
| Model and generation settings | `ai/api/vertex.mjs` | `gemini-pro`, see above |
| AI backend | `variables.mjs` → `currentAiApiName` | `vertex` (loads `ai/api/<name>.mjs`) |
| App and cache name | `variables.mjs` → `app.name` | `fusion-hub` |
| Rate limits | `api/default-config.mjs` → `settings` | 2 concurrent, 2 s, 2 min |
| Opening AI instruction | `ai/instructions.mjs` | English-only |

## Files and storage

Everything is stored under **`~/.fusion-hub/`**:

| Path | Contents |
|---|---|
| `.fusion-hub-cli-history` | History for the main prompt. |
| `.fusion-hub-cmd-mode-history` | History for command mode. |
| `fusion-hub/histories/.ai-chat-history.json[.<sessionId>]` | The current conversation(s). |
| `fusion-hub/histories/<ai-named>.json` | Archived conversations. |
| `fusion-hub/histories/.<name>` | The command-mode history saved with each archive. |
| `fusion-hub/histories/*.bak`, `*.old0-9`, `*.old` | Backups made by `start old`, plus the rotation counter. |
| `fusion-hub/templates/` | Copies used by `start template`. |

`~`, `~~` and `~~~` in file arguments expand to your home directory, `~/.fusion-hub` and `~/.fusion-hub/fusion-hub/histories` respectively.

## Project layout

```
main.mjs              entry: { init(initialInput), executeCommand, cli }
cli.mjs               readline loop, command dispatch, command mode, output helpers, errors
setup.mjs             creates storage dirs, global uncaught-error handler
functions.mjs         path expansion, IDs, JSON streaming helpers, history I/O, timestamps
paths.mjs             storage paths and command module roots
variables.mjs         app name, cache dirs, active AI backend
colors.mjs            ANSI color codes
default-config.mjs    copy of api/default-config.mjs (unused by api/main.mjs)
command/              one module per command (help, start, chat-with-ai, scenario, exit)
ai/api/vertex.mjs     Vertex AI backend: init, chat (streaming), undo, recap, clean, history files
ai/instructions.mjs   prompt text (opening instruction, archive filename prompt)
api/main.mjs          Express server that turns routes into commands
api/default-config.mjs API settings and routes
testing/              runners and curl scripts
```

## Programmatic use

```js
import fusionHub from 'fusion-hub';
await fusionHub.init('start scenario');                        // interactive CLI
await fusionHub.executeCommand('chat-with-ai Hi there');       // one-off command

import api from 'fusion-hub/api';
await api.preconfigure();   // new chat session
await api.init();           // start Express
```

`executeCommand(input, settings, extensiveInputBuffer)` passes `settings` through to the command. Commands read the following from it:

- `aiChatSettings`, such as `chatStdout`, `chatConsole`, `sessionId` and `isPersistent`
- `initParameters`
- `isNew`

## Writing a new command

Create `command/<id>.mjs`:

```js
import cli from 'fusion-hub/cli';

export const command = {
  id: 'hello',
  name: 'hello',
  description: 'Say hello.',
  examples: 'hello world',
  isInputCaseSensitive: false,
  isStartable: false            // true => usable with `start` and needs a start() export
};

export async function process({ input, joinedInput, settings }) {
  cli.output.log(`Hello, ${joinedInput || 'there'}!`);
}

// optional, for startable commands:
// export async function start(...args) { return true; }
// start.new = async (...args) => true;   // `start new hello`
```

The command shows up in `help` automatically. When a `start` function returns `true`, *"<Name> started."* is printed; if it returns a string instead, *"<String> started."* is printed.

## Known quirks

- **`express`** is used by the API but isn't listed in `package.json`.
- The `scenario` command's name and description are copied from `chat-with-ai`, so `help` describes it incorrectly.
- **`playersCount`** is hardcoded to 2. The `game` object in `scenario.mjs` is unused.
- In `api/main.mjs`, a request that gets the `429` response keeps processing after `res.end()`.
- `start.new` archives the default session file even when another `sessionId` is in use.
- In `start old` with a template, `cli.templateNotFoundError` should be `cli.errors.templateNotFoundError`.
- In `chat-with-ai`'s `process`, `isNew` is reassigned even though it's a destructured `const`, so it throws if called with no input.
- In `clean()`, the loop guard uses `|| undoCount>undoCountLimit`; it should be `&&`.
- The Vertex SDK is pulled from GitHub, and `gemini-pro` is a legacy model ID.
