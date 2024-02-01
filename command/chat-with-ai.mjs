import fs from "fs/promises";
import _fs from "fs";
import instructions from 'fusion-hub/ai/instructions';
import { currentAiApiName } from "fusion-hub/variables";
import { getCurrentChatHistoryFilePath, getChatHistoryDirPath, getCommandModeHistoryFilePath } from "fusion-hub/paths";
import { expandFilePath, extractJSONFilename, addPeriodToBasename  } from "fusion-hub/functions";
import setup from "fusion-hub/setup";
import cli, { output } from "fusion-hub/cli";
import path from 'path';

export const command = {
  id: "chat-with-ai",
  name: "chat with AI",
  description: "Send a message to the AI. Returns the AI's response.",
  examples: `chat-with-ai Hello, Mr. AI!`,
  isInputCaseSensitive: true,
  isStartable: true
};

setup.settings['current-input-history-file-path'] ||= null;

const cliErrors = cli.errors;
const ai = await import('fusion-hub/ai/api/'+currentAiApiName);
ai.setCurrentChatHistory((await getCurrentChatHistoryFilePath()));

export async function start({
  reinitialize = false,
  action = undefined,
  inputHistoryFilePath = undefined,
  outputHistoryFilePath = undefined,
  startNewIfInitializationFailed = true,
  initParameters = {}
} = {}) {
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  outputHistoryFilePath ||= await getCurrentChatHistoryFilePath();
  if (typeof initParameters !== 'object') {
    cliErrors.noArgumentsAllowedError();
    return false;
  }
  const { sessionId = 0 } = initParameters;
  initParameters.isClearingMemory = false;
  if (ai.verifyCurrentChatHistoryExists()) action ||= "Resuming";
  if (!ai.checkInitialized(sessionId))
    await ai.init({ action, inputHistoryFilePath, outputHistoryFilePath, ...initParameters });
  else if (reinitialize) {
    action ||= "Re-initializing";
    await ai.init({ action, inputHistoryFilePath, outputHistoryFilePath, ...initParameters });
  }
  if (!ai.checkInitialized(sessionId) && startNewIfInitializationFailed)
    start.new(null, initParameters);
  return true;
}

start.new = async (
  filePath = null,
  initParameters = {},
  startType = 'new'
) => {
  const { log, warn } = output;
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  const { isPersistent = true } = initParameters;
  if (typeof initParameters !== 'object') {
    cliErrors.argumentsLimitError(1);
    return false;
  }
  const { sessionId = 0 } = initParameters;
  if (ai.verifyCurrentChatHistoryExists(sessionId) && isPersistent) {
    const temporaryHistoryFilename = `${new Date()
      .toISOString()
      .replace(/:/g, "-")
      .slice(0, 19)}.json`;

    const temporaryHistoryFilePath = path.join((await getChatHistoryDirPath()),temporaryHistoryFilename);
    await fs.rename(ai.getCurrentChatHistoryFilename(sessionId), temporaryHistoryFilePath);
    await start({
      action: "Finalizing previous",
      inputHistoryFilePath: temporaryHistoryFilePath,
      startNewIfInitializationFailed: false
    });
    let suggestedFilenameSource, suggestedFilename, suggestedFilePath;
    if (setup.settings['current-input-history-file-path']==null) {
      suggestedFilenameSource = await ai.chat(
        instructions.aiInstructionsForFilenameCreation(),
        { chatStdout: { write: () => {} }, isPersistent: false }
      );
      suggestedFilename = extractJSONFilename(suggestedFilenameSource) || temporaryHistoryFilename;
    }
    else {
      suggestedFilePath = setup.settings['current-input-history-file-path'];
      suggestedFilename = path.basename(suggestedFilePath);
    }
    const chatHistoryDirPath = await getChatHistoryDirPath();
    let suggestedFilePathOriginal =  path.join(chatHistoryDirPath,suggestedFilename);
    suggestedFilePath = suggestedFilePathOriginal
    let copyNum=2;
    while (_fs.existsSync(suggestedFilePath+".json") || copyNum > 100) {
      suggestedFilePath = suggestedFilePathOriginal.replace(/\.json$/g,'')+` (${copyNum}).json`;
      copyNum++;
    }
    if (copyNum>100) {
      warn("\nWARNING: The AI is suggesting the same filename, but 99 copies were found. This is excessive; reverting to timestamps.\n")
      suggestedFilename=temporaryHistoryFilename;
      suggestedFilePath=temporaryHistoryFilePath;
    } else
      suggestedFilename = path.basename(suggestedFilePath);
      if (startType==='specified') {
        log(
          `Current conversation will be saved as: ${suggestedFilename}`
        );
      } else {
        log(
          `Previous conversation saved as: ${suggestedFilename}`
        );
      }
    const newCommandModeHistoryFilePath = addPeriodToBasename(suggestedFilePath.replace(/\.json$/g,''));
    const commandModeHistoryFilePath = await getCommandModeHistoryFilePath();
    if (_fs.existsSync(commandModeHistoryFilePath))
      await fs.rename(commandModeHistoryFilePath, newCommandModeHistoryFilePath);
    if (suggestedFilename != temporaryHistoryFilename)
      await fs.rename(
        temporaryHistoryFilePath,
        suggestedFilePath
      );
  }
  if (startType=='new')
    setup.settings['current-input-history-file-path']=null;
  if (startType!='specified')
    await ai.init({ action: "Starting "+startType, ...initParameters });
  if (filePath)
  {
    setup.settings['current-input-history-file-path'] = filePath;
    await setup.pushSetupSettings();
    return await start.new(null, {}, 'specified');
  }
  return true;
}

start.old = async (historyJsonFilePath=null, 
  initParameters = {}, startType='old'
) => {
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  initParameters.isClearingMemory = false;
  if (!historyJsonFilePath) {
    cliErrors.noArgumentsSuppliedError(1);
    return false;
  }
  if (typeof initParameters !== 'object') {
    cliErrors.argumentsLimitError(1); 
    return false;
  }

  let prefix='';
  if (!historyJsonFilePath.match(/^[\.\/\~]/g))
    prefix=`~~~/`

  const inputHistoryFilePathSource = await expandFilePath(prefix+historyJsonFilePath);

  if (startType=='template') {
    initParameters.inputHistoryFilePath = await expandFilePath(`~~~/${path.basename(historyJsonFilePath)}`, { contextDir: 'templates' });
    if (_fs.existsSync(inputHistoryFilePathSource))
      await fs.copyFile(inputHistoryFilePathSource, initParameters.inputHistoryFilePath);
    else
      cli.templateNotFoundError();
  }
  else {
    initParameters.inputHistoryFilePath = inputHistoryFilePathSource;
  }

  const { inputHistoryFilePath } = initParameters;

  if (startType=='template' && (!_fs.existsSync(inputHistoryFilePath))) {
    if (_fs.existsSync(inputHistoryFilePathSource))
      await fs.copyFile(inputHistoryFilePathSource, inputHistoryFilePath);
    else
      cli.templateNotFoundError();
  }

  if (_fs.existsSync(inputHistoryFilePath)) {
    await fs.copyFile(inputHistoryFilePath, inputHistoryFilePath+".bak");
    let oldCopy=1, L=10;
    if (_fs.existsSync(inputHistoryFilePath+".old")) {
      oldCopy = Number(await fs.readFile(inputHistoryFilePath+".old", "utf-8"));
    }
    await fs.copyFile(inputHistoryFilePath, inputHistoryFilePath+".old"+oldCopy%L);
    await fs.writeFile(inputHistoryFilePath+".old", `${oldCopy++}`);
  }
  await start.new(null, initParameters, startType);
  const oldCommandModeHistoryFilePath = addPeriodToBasename(inputHistoryFilePath.replace(/\.json$/g,''));
  const oldCommandModeHistoryFilePathSource = addPeriodToBasename(inputHistoryFilePathSource.replace(/\.json$/g,''));
  const commadeModeHistoryFilePath = await getCommandModeHistoryFilePath();
  if (_fs.existsSync(oldCommandModeHistoryFilePath))
    await fs.copyFile(oldCommandModeHistoryFilePath, commadeModeHistoryFilePath);
  else if (_fs.existsSync(oldCommandModeHistoryFilePathSource))
    await fs.copyFile(oldCommandModeHistoryFilePathSource, commadeModeHistoryFilePath);
  if (startType=='template')
    setup.settings['current-input-history-file-path'] = null;
  else
    setup.settings['current-input-history-file-path'] = inputHistoryFilePath;
  return true;
}

start.last = async (initParameters = {}
) => {
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  const { sessionId = 0 } = initParameters;
  const result = await ai.undo({sessionId, showLastContent: true, chatConsole: cli.output});
  if (result && result.length > 0) {
    return true;
  }
  return false;
}

start.recap = async (initParameters = {}
  ) => {
    initParameters.newDefaultStdout ||= cli.output;
    initParameters.newDefaultConsole ||= cli.output;
    const { sessionId = 0 } = initParameters;
    const result = await ai.recap({sessionId, showLastContent: true, chatConsole: cli.output});
    if (result && result.length > 0) {
      return true;
    }
    return false;
}

start.send = async (filename, initParameters = {}
  ) => {
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  const fileData = await fs.readFile(filename, "utf-8");
  await start({ initParameters });
  const result = await ai.chat(fileData, { chatStdout: initParameters.newDefaultStdout, chatConsole: initParameters.newDefaultConsole });
  if (result && result.length > 0) {
    return true;
  }
  return false;
}

start.template = async (templateSourceJsonFilePath=null, initParameters = {}
  ) => {
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  const result = await start.old(templateSourceJsonFilePath, initParameters, 'template');
  if (result) {
    return true;
  }
  return false;
}

export const { chat } = ai;

export async function process({ input, joinedInput: aiChatInput, settings = {} }) {
  const { aiChatSettings, initParameters = {}, isNew = false } = settings;
  initParameters.newDefaultStdout ||= cli.output;
  initParameters.newDefaultConsole ||= cli.output;
  if (input.length==0)
    isNew=true;
  if (isNew)
    await start.new(null, { ...initParameters });
  else
    await start({initParameters});
  if (aiChatInput)
    return await ai.chat(aiChatInput, aiChatSettings);
}