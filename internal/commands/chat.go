package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
)

func chatWithAI() *Command {
	return &Command{
		ID:            "chat-with-ai",
		Name:          "chat with AI",
		Description:   "Send a message to the AI and stream its response. With no message, start a new chat.",
		Examples:      "chat-with-ai Hello, Mr. AI!",
		CaseSensitive: true,
		Start: map[string]StartFunc{
			"":              startChat,
			"new":           startNewChat,
			"old":           startOldChat,
			"last":          startUndo,
			"undo":          startUndo,
			"recap":         startRecap,
			"send":          startSend,
			"templated":     startTemplated,
			"new-templated": startNewTemplated,
		},
		Run: func(ctx context.Context, env *Env, in Input) (string, error) {
			if in.Raw == "" {
				_, err := newChat(ctx, env, "")
				return "", err
			}
			return env.Chat.Send(ctx, env.SessionID, in.Raw, env.Out, chat.SendOptions{})
		},
	}
}

func startChat(_ context.Context, env *Env, _ []string) (string, error) {
	if env.Chat.Exists(env.SessionID) {
		env.Out.Log("Resuming chat with AI...")
	} else {
		env.Out.Log("Starting new chat with AI...")
	}
	env.Chat.Session(env.SessionID)
	return "", nil
}

func newChat(ctx context.Context, env *Env, saveAs string) (string, error) {
	archived, err := env.Chat.New(ctx, env.SessionID, saveAs)
	if err != nil {
		return "", err
	}
	if archived != "" {
		env.Out.Log("Previous conversation saved as: %s", filepath.Base(archived))
	}
	if saveAs != "" {
		env.Out.Log("Current conversation will be saved as: %s", filepath.Base(saveAs))
	}
	return archived, nil
}

func startNewChat(ctx context.Context, env *Env, args []string) (string, error) {
	saveAs := ""
	if len(args) > 1 {
		return "", errors.New("Command limited to 1 argument.")
	}
	if len(args) == 1 {
		saveAs = args[0]
	}
	_, err := newChat(ctx, env, saveAs)
	return "", err
}

func startOldChat(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("Command takes 1 argument: the conversation file.")
	}
	archived, err := env.Chat.Old(ctx, env.SessionID, args[0], false)
	if err != nil {
		return "", err
	}
	if archived != "" {
		env.Out.Log("Previous conversation saved as: %s", filepath.Base(archived))
	}
	return "", nil
}

func printExchange(env *Env, user, assistant string) {
	env.Out.Print("----------------")
	env.Out.Print("user-chat-input>  " + user)
	env.Out.Print("----------------")
	env.Out.Print(assistant)
}

func startUndo(_ context.Context, env *Env, _ []string) (string, error) {
	user, assistant, ok, err := env.Chat.Undo(env.SessionID)
	if err != nil {
		return "", err
	}
	if ok {
		printExchange(env, user, assistant)
	} else {
		env.Out.Print("(The conversation is now empty.)")
	}
	return "", nil
}

func startRecap(_ context.Context, env *Env, _ []string) (string, error) {
	user, assistant, ok := env.Chat.Recap(env.SessionID)
	if !ok {
		return "", errors.New("Nothing to recap yet.")
	}
	printExchange(env, user, assistant)
	return "", nil
}

func startSend(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("Command takes 1 argument: the file to send.")
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return "", err
	}
	_, err = env.Chat.Send(ctx, env.SessionID, string(b), env.Out, chat.SendOptions{})
	return "", err
}

func startTemplated(ctx context.Context, env *Env, args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("Command takes 1 argument: the template name.")
	}
	archived, err := env.Chat.Old(ctx, env.SessionID, args[0], true)
	if err != nil {
		return "", err
	}
	if archived != "" {
		env.Out.Log("Previous conversation saved as: %s", filepath.Base(archived))
	}
	return "", nil
}

func startNewTemplated(_ context.Context, env *Env, args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("Command takes 1 argument: the template name.")
	}
	path, err := env.Chat.SaveTemplate(env.SessionID, args[0])
	if err != nil {
		return "", err
	}
	env.Out.Log("Template created: %s", path)
	return "", nil
}
