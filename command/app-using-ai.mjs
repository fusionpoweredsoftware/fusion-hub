import _fs from "fs";
import fs from "fs/promises";
import { start as aiStart } from 'fusion-hub/command/chat-with-ai'
import { process as mcProcess } from 'fusion-hub/command/multi-command'
import cli, { output, properties as cliProperties } from 'fusion-hub/cli';

import {
    currentAiApiName
} from "fusion-hub/variables";
import {
    getArgs,
    isString,
    expandFilePath
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

setup.settings[".current-app-config"] = {};

const ai = await import('fusion-hub/ai/api/' + currentAiApiName);
ai.setCurrentChatHistory((await getCurrentChatHistoryFilePath()));

const cliErrors = cli.errors;
const runApp = async (
    appFilePath,
    appInput,
    multiCommandSettings={},
    isDebugging=false
  ) => {
    if (!isDebugging) {
        multiCommandSettings.aiChatSettings ||= {};
        multiCommandSettings.aiChatSettings.chatStdout ||= { write: () => {} };
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
    if (isString(appSettings))
        appSettings = json5.parse(appSettings);
    const { sessionId = 0 } = (appSettings ||= {});
    if (appFilePath)
        appFilePath+=".app.ail.js";
    setup.settings[".current-app-config"][sessionId] ||={};
    Object.assign(setup.settings[".current-app-config"][sessionId], {
        appFilePath: appFilePath || setup.settings[".current-app-config"][sessionId].appFilePath || null,
        appSettings: Object.assign(appSettings, setup.settings[".current-app-config"][sessionId].appSettings || {}),
        isDebugging: false
    });
    await setup.pushSetupSettings();
    return await aiStart();
}

start.debug = async (appFilePath = null,
    appSettings = null,
    parameterOverload = {}) => {

    if (typeof parameterOverload !== 'object') {
        cliErrors.noArgumentsAllowedError();
        return false;
    }
    if (isString(appSettings))
        appSettings = json5.parse(appSettings);
    const { sessionId = 0 } = (appSettings ||= {});
    if (appFilePath)
        appFilePath+=".app.ail.js";
    setup.settings[".current-app-config"][sessionId] ||={};
    Object.assign(setup.settings[".current-app-config"][sessionId], {
        appFilePath: appFilePath || setup.settings[".current-app-config"][sessionId].appFilePath || null,
        appSettings: Object.assign(appSettings, setup.settings[".current-app-config"][sessionId].appSettings || {}),
        isDebugging: false
    });
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
    if (isString(appSettings))
        appSettings = json5.parse(appSettings);
    const { sessionId = 0 } = (appSettings ||= {});
    if (appFilePath)
        appFilePath+=".app.ail.js";
    setup.settings[".current-app-config"][sessionId] ||={};
    setup.settings[".current-app-config"][sessionId] = {
        appFilePath: appFilePath || setup.settings[".current-app-config"][sessionId].appFilePath || null,
        appSettings: Object.assign(appSettings, setup.settings[".current-app-config"][sessionId].appSettings || {}),
        isDebugging: setup.settings[".current-app-config"][sessionId].isDebugging || false
    };
    await setup.pushSetupSettings();
    const result = await aiStart.new(historyJsonFilePath);
    setup.settings[".current-app-config"][sessionId].aiChatSettings ||= {};
    setup.settings[".current-app-config"][sessionId].aiChatSettings.chatHistory = [];
    return result;
}

start.old = async (
    appFilePath = null,
    appSettings = null,
    historyJsonFilePath = null,
    parameterOverload = {}) => {
    if (isString(appSettings))
        appSettings = json5.parse(appSettings);
    const { sessionId = 0 } = (appSettings ||= {});
    if (typeof parameterOverload !== 'object') {
        cliErrors.argumentsLimitError(3);
        return false;
    }
    if (appFilePath)
        appFilePath+=".app.ail.js";
    setup.settings[".current-app-config"][sessionId] ||={};
    setup.settings[".current-app-config"][sessionId] = {
        appFilePath: appFilePath || setup.settings[".current-app-config"][sessionId].appFilePath || null,
        appSettings: Object.assign(appSettings, setup.settings[".current-app-config"][sessionId].appSettings || {}),
        isDebugging: setup.settings[".current-app-config"][sessionId].isDebugging || false
    };
    await setup.pushSetupSettings();
    const result = await aiStart.old(historyJsonFilePath);
    setup.settings[".current-app-config"][sessionId].aiChatSettings ||= {};
    const historyFilePath = await expandFilePath(historyJsonFilePath, {contextDir: "histories"});
    const chatHistoryToJSON = JSON.parse(await fs.readFile(historyFilePath, 'utf-8'));
    setup.settings[".current-app-config"][sessionId].aiChatSettings.chatHistory = chatHistoryToJSON;
    return result;
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
   const { log, error } = output;
   try {
    if (sessionId in queue) {
        log("App already running.");
        return;
    }
    queue[sessionId] = 1;
    setup.settings[".current-app-config"][sessionId] ||= {};
    setup.settings[".current-app-config"][sessionId] ||= {
            appFilePath: setup.settings[".current-app-config"][sessionId].appFilePath || null,
            appSettings: setup.settings[".current-app-config"][sessionId].appSettings || {},
            isDebugging: setup.settings[".current-app-config"][sessionId].isDebugging || false
    };

    const appConfig = setup.settings[".current-app-config"][sessionId];
    const multiCommandSettings = JSON.parse(JSON.stringify(appConfig));
    const { isDebugging } = setup.settings[".current-app-config"][sessionId];
    
    multiCommandSettings.aiChatSettings=setup.settings[".current-app-config"][sessionId].aiChatSettings
    multiCommandSettings.aiChatSettings ||= {};
    multiCommandSettings.aiChatSettings.sessionId = sessionId;
    multiCommandSettings.app ||= {};
    multiCommandSettings.app.input = appInput || '';
    multiCommandSettings.app.settings = multiCommandSettings.appSettings || {}; 
    delete multiCommandSettings.appSettings;  //no longer needed: moved to multiCommandSettings.app.settings
    multiCommandSettings.contextDir ||= 'app';
    multiCommandSettings.app.filePath = multiCommandSettings.appFilePath;
    delete multiCommandSettings.appFilePath;
    Object.assign(multiCommandSettings.app, settings); //no longer need: moved to multiCommandSettings.app.filePath
    cliProperties.commandMode = null;
    
    await runApp(appConfig.appFilePath, appInput, multiCommandSettings, isDebugging);

   } catch (err) {
        error(err);
   }
   cliProperties.commandMode = command.id;
   delete queue[sessionId];
}