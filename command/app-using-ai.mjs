import _fs from "fs";
import { start as aiStart } from 'fusion-hub/command/chat-with-ai'
import { process as mcProcess } from 'fusion-hub/command/multi-command'
import {
    currentAiApiName
} from "fusion-hub/variables";
import {
    getCurrentChatHistoryFilePath
} from "fusion-hub/paths";
import setup from "fusion-hub/setup";

export const command = {
    id: "app-using-ai",
    name: "app using AI",
    description: "Send a message to the AI, filtered. Returns the AI's response.",
    examples: `app-using-ai myFirst.app.ail.js`,
    isInputCaseSensitive: true,
    isStartable: true
};

setup.settings['current-input-history-file-path'] ||= null;

const ai = await import('fusion-hub/ai/api/' + currentAiApiName);
ai.setCurrentChatHistory((await getCurrentChatHistoryFilePath()));

export async function start({
    reinitialize = false,
    action = undefined,
    inputHistoryFilePath = undefined,
    outputHistoryFilePath = undefined,
    startNewIfInitilizationFailed = true,
    initParameters = {}
} = {}) {
    return await aiStart({reinitialize, action, inputHistoryFilePath, outputHistoryFilePath, startNewIfInitilizationFailed, initParameters});
}

start.new = async (
    ailFileName = null,
    chatHistoryFileName = null,
    initParameters = {},
    startType = 'new') => {
    if (typeof initParameters !== 'object') {
        cliErrors.argumentsLimitError(2);
        return false;
    }
    return await aiStart.new(chatHistoryFileName, initParameters, startType) && mcProcess(ailFileName);
}

start.old = async (historyJsonFilePath = null,
    initParameters = {}, startType = 'old'
) => {
    if (typeof initParameters !== 'object') {
        cliErrors.argumentsLimitError(1);
        return false;
    }
    return await aiStart.new(historyJsonFilePath, initParameters, startType);
}

start.last = async (initParameters = {}) => {
    if (typeof initParameters !== 'object') {
        cliErrors.noArgumentsAllowedError();
        return false;
    }
  return await aiStart.last(initParameters);
}

start.recap = async (initParameters = {}) => {
  return await aiStart.last(initParameters);
}

start.send = async (filename, initParameters = {}) => {
  return await aiStart.last(filename, initParameters);
}

start.template = async (templateSourceJsonFilePath = null, initParameters = {}) => {
  return await aiStart.last(templateSourceJsonFilePath, initParameters);
}

export const {
    chat
} = ai;

export async function process({
    input,
    joinedInput,
    settings = {}
}) {
   await mcProcess({ input, joinedInput });
}