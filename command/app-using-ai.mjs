import _fs from "fs";
import { start as aiStart } from 'fusion-hub/command/chat-with-ai'
import { process as mcProcess } from 'fusion-hub/command/multi-command'
import { error, log, properties as cliProperties } from 'fusion-hub/cli';

import {
    currentAiApiName
} from "fusion-hub/variables";
import {
    getArgs
} from "fusion-hub/functions"
import {
    getCurrentChatHistoryFilePath
} from "fusion-hub/paths";
import setup from "fusion-hub/setup";
import json5 from 'json5';

export const command = {
    id: "app-using-ai",
    name: "app using AI",
    description: "Send a message to the AI, filtered. Returns the AI's response.",
    examples: `app-using-ai myFirst.app.ail.js`,
    isInputCaseSensitive: true,
    isStartable: true
};

setup.settings['current-app-config'] ||= {};

const ai = await import('fusion-hub/ai/api/' + currentAiApiName);
ai.setCurrentChatHistory((await getCurrentChatHistoryFilePath()));


const runApp = async (
    appFilePath,
    appInput,
    appSettings,
    isDebugging=false
  ) => {
    if (!isDebugging) {
        appSettings.aiChatSettings = { chatStdout: { write: () => {} } };
        appSettings.isLoggingMultiCommandFileContents = (appSettings.isLoggingMultiCommandFileContents!=undefined)  || false;
        appSettings.isInterruptable = (appSettings.isInterruptable!=undefined) || false;
        appSettings.isVerbose = false;
    }
    
    await mcProcess({
      input: [appFilePath, ...getArgs(appInput)],
      joinedInput: (appFilePath + " " + appInput).trim(),
      settings: appSettings,
    });
  }


export async function start(appFilePath = null,
    appSettings = null,
    parameterOverload = {},
{
} = {}) {
    if (typeof parameterOverload !== 'object') {
        cliErrors.argumentsLimitError(2);
        return false;
    }
    setup.settings['current-app-config'] = {
        appFilePath: appFilePath || setup.settings['current-app-config'].appFilePath || null,
        settings: json5.parse(appSettings) || setup.settings['current-app-config'].settings || {},
        isDebugging: setup.settings["current-app-config"].isDebugging || false
    };
    await setup.pushSetupSettings();
    return await aiStart();
}

start.debug = async ({
    parameterOverload = {},
} = {}) => {
    if (typeof parameterOverload !== 'object') {
        cliErrors.noArgumentsAllowedError();
        return false;
    }

    return await aiStart();
}

start.new = async (
    appFilePath = null,
    historyJsonFilePath = null,
    appSettings = null,
    parameterOverload = {}) => {
    if (typeof parameterOverload !== 'object') {
        cliErrors.argumentsLimitError(3);
        return false;
    }
    setup.settings['current-app-config'] = {
        appFilePath: appFilePath || setup.settings['current-app-config'].appFilePath || null,
        settings: json5.parse(appSettings) || setup.settings['current-app-config'].settings || {},
        isDebugging: setup.settings["current-app-config"].isDebugging || false
    };
    await setup.pushSetupSettings();
    return await aiStart.new(historyJsonFilePath);
}

start.old = async (
    appFilePath = null,
    historyJsonFilePath = null,
    appSettings = null,
    parameterOverload = {}) => {
    if (typeof parameterOverload !== 'object') {
        cliErrors.argumentsLimitError(3);
        return false;
    }
    setup.settings['current-app-config'] = {
        appFilePath: appFilePath || setup.settings['current-app-config'].appFilePath || null,
        settings: json5.parse(appSettings) || setup.settings['current-app-config'].settings || {},
        isDebugging: setup.settings["current-app-config"].isDebugging || false
    };
    await setup.pushSetupSettings();
    return await aiStart.new(historyJsonFilePath, undefined, startType);
}

export const {
    chat
} = ai;


export const queue = {};

export async function process({
    joinedInput: appInput,
    settings = {},
}) { 
   const { sessionId = 0 } = settings;
   try {
    if (sessionId in queue) {
        log("App already running.");
        return;
    }
    queue[sessionId] = 1;
    setup.settings['current-app-config'] = {
            appFilePath: setup.settings['current-app-config'].appFilePath || null,
            settings: setup.settings['current-app-config'].settings || {},
            isDebugging: setup.settings["current-app-config"].isDebugging || false
    };

    const appConfig = setup.settings['current-app-config'];
    const appSettings = JSON.parse(JSON.stringify(appConfig));
    const { isDebugging } = setup.settings['current-app-config'];
    
    appSettings.aiChatSettings ||= {};
    appSettings.aiChatSettings.sessionId = sessionId;
    appSettings.app ||= {};
    appSettings.app.input = appInput;
    Object.assign(appSettings, settings);
    cliProperties.commandMode = null;
    
    await runApp(appConfig.appFilePath, appInput, appSettings, isDebugging);

   } catch (err) {
        error(err);
   }
   cliProperties.commandMode = command.id;
   delete queue[sessionId];
}