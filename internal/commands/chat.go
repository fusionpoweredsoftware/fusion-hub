package commands

import (
	"context"
	"errors"
	"os"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
)

// errOneArgument is the original's noArgumentsSuppliedError(1).
var errOneArgument = errors.New("Command takes 1 argument.")

func chatWithAI() *Command {
	return &Command{
		ID:            "chat-with-ai",
		Name:          "chat with AI",
		Description:   "Send a message to the AI. Returns the AI's response.",
		Examples:      "chat-with-ai Hello, Mr. AI!",
		CaseSensitive: true,
		Start: map[string]StartFunc{
			"":              startChat,
			"new":           startNewChat,
			"old":           startOldChat,
			"last":          startLast,
			"recap":         startRecap,
			"send":          startSend,
			"templated":     startTemplated,
			"new-templated": startNewTemplated,
		},
		Run: func(ctx context.Context, env *Env, in Input) (string, error) {
			if in.Raw == "" {
				return "", env.Chat.New(ctx, env.SessionID, "")
			}
			_, err := env.Chat.Send(ctx, env.SessionID, in.Raw, env.Out, chat.SendOptions{})
			return "", err
		},
	}
}

func startChat(ctx context.Context, env *Env, _ []string) (string, error) {
	return "", env.Chat.Start(ctx, env.SessionID)
}

// newChat starts a new conversation (archiving the current one), saving it
// as name later when name is given. app.go uses it for "start new".
func newChat(ctx context.Context, env *Env, name string) (string, error) {
	return "", env.Chat.New(ctx, env.SessionID, name)
}

func startNewChat(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New("Command limited to 1 argument.")
	}
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	return newChat(ctx, env, name)
}

func startOldChat(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) == 0 {
		return "", errOneArgument
	}
	return "", env.Chat.Old(ctx, env.SessionID, args[0])
}

// logExchange prints a user input and a reply the way the original's
// recap did, through the timestamped logger.
func logExchange(env *Env, user, reply string) {
	env.Out.Log("----------------")
	env.Out.Log("user-chat-input>  %s", user)
	env.Out.Log("----------------")
	env.Out.Log("%s", reply)
}

func startLast(_ context.Context, env *Env, _ []string) (string, error) {
	user, prev, ok, err := env.Chat.Undo(env.SessionID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotStarted
	}
	logExchange(env, user, prev)
	return "", nil
}

func startRecap(_ context.Context, env *Env, _ []string) (string, error) {
	user, reply, ok := env.Chat.Recap(env.SessionID)
	if !ok {
		return "", ErrNotStarted
	}
	logExchange(env, user, reply)
	return "", nil
}

func startSend(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) == 0 {
		return "", errOneArgument
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return "", err
	}
	// Send starts the session first (Initializing/Resuming, instructions).
	reply, err := env.Chat.Send(ctx, env.SessionID, string(b), env.Out, chat.SendOptions{})
	if err != nil {
		return "", err
	}
	if reply == "" {
		return "", ErrNotStarted
	}
	return "", nil
}

func startTemplated(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) == 0 {
		return "", errOneArgument
	}
	return "", env.Chat.Templated(ctx, env.SessionID, args[0])
}

func startNewTemplated(_ context.Context, env *Env, args []string) (string, error) {
	if len(args) == 0 {
		return "", errOneArgument
	}
	_, err := env.Chat.NewTemplate(env.SessionID, args[0])
	return "", err
}
