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
    multiCommandSettings,
    isDebugging=false
  ) => {
    if (!isDebugging) {
        multiCommandSettings.aiChatSettings = { chatStdout: { write: () => {} } };
        multiCommandSettings.isLoggingMultiCommandFileContents = (multiCommandSettings.isLoggingMultiCommandFileContents!=undefined)  || false;
        multiCommandSettings.isInterruptable = (multiCommandSettings.isInterruptable!=undefined) || false;
        multiCommandSettings.isVerbose = false;
    }
    
    await mcProcess({
      input: [appFilePath, ...getArgs(appInput)],
      joinedInput: (appFilePath + " " + appInput).trim(),
      settings: multiCommandSettings,
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
        appSettings: json5.parse(appSettings) || setup.settings['current-app-config'].appSettings || {},
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
    appSettings = null,
    historyJsonFilePath = null,
    parameterOverload = {}) => {
    if (typeof parameterOverload !== 'object') {
        cliErrors.argumentsLimitError(3);
        return false;
    }
    if (appFilePath)
        appFilePath+=".app.ail.js";
    setup.settings['current-app-config'] = {
        appFilePath: appFilePath || setup.settings['current-app-config'].appFilePath || null,
        appSettings: json5.parse(appSettings) || setup.settings['current-app-config'].appSettings || {},
        isDebugging: setup.settings["current-app-config"].isDebugging || false
    };
    await setup.pushSetupSettings();
    return await aiStart.new(historyJsonFilePath);
}

start.old = async (
    appFilePath = null,
    appSettings = null,
    historyJsonFilePath = null,
    parameterOverload = {}) => {
    if (typeof parameterOverload !== 'object') {
        cliErrors.argumentsLimitError(3);
        return false;
    }
    setup.settings['current-app-config'] = {
        appFilePath: appFilePath || setup.settings['current-app-config'].appFilePath || null,
        appSettings: json5.parse(appSettings) || setup.settings['current-app-config'].appSettings || {},
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
            appSettings: setup.settings['current-app-config'].appSettings || {},
            isDebugging: setup.settings["current-app-config"].isDebugging || false
    };

    const appConfig = setup.settings['current-app-config'];
    const multiCommandSettings = JSON.parse(JSON.stringify(appConfig));
    const { isDebugging } = setup.settings['current-app-config'];
    
    multiCommandSettings.aiChatSettings ||= {};
    multiCommandSettings.aiChatSettings.sessionId = sessionId;
    multiCommandSettings.app ||= {};
    multiCommandSettings.app.input = appInput || '';
    multiCommandSettings.app.settings = multiCommandSettings.appSettings || {}; 
    multiCommandSettings.contextDir ||= 'app';
    delete multiCommandSettings.appSettings;
    multiCommandSettings.app.filePath = multiCommandSettings.appFilePath;
    delete multiCommandSettings.appFilePath;
    Object.assign(multiCommandSettings.app, settings);
    cliProperties.commandMode = null;
    
    await runApp(appConfig.appFilePath, appInput, multiCommandSettings, isDebugging);

   } catch (err) {
        error(err);
   }
   cliProperties.commandMode = command.id;
   delete queue[sessionId];
}