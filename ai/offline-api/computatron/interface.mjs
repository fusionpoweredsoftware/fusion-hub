'use strict';
import instructions from 'ai/instructions';
import setup from 'ai/setup';
import { emulateVertexGenerativeModel } from './dependencies/emulator.mjs';

const {
  emitWarning
} = process;

process.emitWarning = (warning, ...args) => {
  if (args[0] === 'ExperimentalWarning') {
    return;
  }

  if (args[0] && typeof args[0] === 'object' && args[0].type === 'ExperimentalWarning') {
    return;
  }

  return emitWarning(warning, ...args);
};

const isObject = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const aiChatsMap = {};

const originalWrite = process.stdout.write.bind(process.stdout);
const fs = await import('fs/promises');
const _fs = await import('fs');

let currentChatHistoryFilePath = './.ai-chat-history.json';
let isInitialized = {};
let defaultConsole = console;
let defaultStdout = process.stdout;
let defaultChatInstructions = instructions.defaultAiInstructionsForConversationStart['computatron'];

export const checkInitialized = (sessionId) => {
  return isInitialized[sessionId];
}

process.stdout.write = (...input) => {
  const writeLog =defaultConsole.log;
  if (input[0].match(/^Error: 429 Too Many Requests\n    at GenerativeModel\.generateContentStream.*/g)) {
    writeLog("\nThe AI is currently busy. Try again later.\n")
  } else {
    originalWrite(...input);
  }
};

let old_project;
let generativeModel = null;

// Instantiate the models
export async function clearMemory(clearMemoryConsole = console, sessionId = 0) {
  try {
    aiChatsMap[sessionId] = await generativeModel.startChat({});
  } catch (err) {
    await clearMemoryConsole.error(err)
  }
}

export const undo = async ({
  sessionId = 0,
  showLastContent = false,
  chatConsole = defaultConsole,
  sliceStart = -3,
  sliceEnd = -2
}) => {
  const history = (await recap({
    sessionId,
    showLastContent,
    chatConsole,
    sliceStart
  })).slice(0, sliceEnd);
  if (history && history.length > 0) {
    aiChatsMap[sessionId].historyInternal.length=0;
    aiChatsMap[sessionId].historyInternal.push(...history);
    return history
  }
  return [];
}

export const recap = async ({
  sessionId = 0,
  showLastContent = true,
  chatConsole = defaultConsole,
  sliceStart = -2
}) => {
  if (!aiChatsMap[sessionId]) {
    await chatConsole.error("ERROR: No data loaded in memory yet.")
    return [];
  }
  let history = aiChatsMap[sessionId].historyInternal;
  const lastStart = history.slice(sliceStart);
  if (showLastContent && lastStart && lastStart.length > 0) {
    const len = lastStart[1].parts.length;
    await chatConsole.log('----------------');
    for (let i = 0; i < len; i++) {
      await chatConsole.log('user-chat-input>  ' + lastStart[1].parts[i].text);
    }
    await chatConsole.log('----------------');
  }

  if (showLastContent && lastStart && lastStart.length > 0) {
    const len = lastStart[0].parts.length;
    for (let i = 0; i < len; i++) {
      await chatConsole.log(lastStart[0].parts[i].text);
    }
  }
  return history;
}

export const clean = async (sessionId = 0, undoCountLimit = 99) => {
  let history = aiChatsMap[sessionId].historyInternal;
  let undoCount = 0;
  while (JSON.stringify(history.slice(-1)) == `[{"role":"model","parts":[{"text":""}]}]` || undoCount>undoCountLimit) {
    await undo({
      sessionId
    });
    history = aiChatsMap[sessionId].historyInternal;
    undoCount++;
  }
  return history;
}

export const chat = async (textInput, {
  chatStdout = defaultStdout,
  isPersistent = true,
  chatConsole = defaultConsole,
  sessionId = 0,
  isInitialization=false,
  secondAttempt=false,
  isStreaming = true,
  chatHistory = undefined,
  channel = 0
} = {}) => {
  try {
    chatHistory||={};
    
    if (chatHistory && isObject(chatHistory) && Array.isArray(chatHistory[channel])) {
      aiChatsMap[sessionId].historyInternal = chatHistory[channel];
    }

    else if (chatHistory && !isObject(chatHistory) && Array.isArray(chatHistory)) {
      chatHistory = { [channel]: chatHistory };
    }

    chatHistory[channel] ||= aiChatsMap[sessionId].historyInternal;
    let freezeHistory = JSON.stringify(chatHistory[channel]);

    await clean(sessionId);
    const userMessage = [{
      text: textInput
    }];
    let streamResult;
    try {
      streamResult = await aiChatsMap[sessionId].sendMessageStream(userMessage);
    } catch (err)
    {
      //await sleep(10000)
      await init({sessionId})
      // Restart connection
      streamResult = await aiChatsMap[sessionId].sendMessageStream(userMessage)
    }
    let finalOutput = '';

    if (isStreaming)
    {
      //await chatStdout.write("\nai> ");
      for await (const item of streamResult.stream) {
          const len = item.candidates[0].content.parts.length;
          for (let i = 0; i < len; i++) {
            const textStream = item.candidates[0].content.parts[i].text;
            finalOutput += textStream;
            await chatStdout.write(textStream);
          }
 
      }
    }
    if (!isStreaming) {
      const awaitedResponse = await streamResult.response;
      const responseText = awaitedResponse.candidates[0].content.parts[0].text;
      finalOutput = responseText;
      if (!finalOutput) {
        aiChatsMap[sessionId].historyInternal.length=0;
        aiChatsMap[sessionId].historyInternal.push(...JSON.parse(freezeHistory));
        if (!secondAttempt && textInput.match(/[^ ]!/g)) {
          finalOutput = await chat(textInput.replace('!',' !'), {
            chatStdout,
            isPersistent,
            chatConsole,
            sessionId,
            isInitialization,
            secondAttempt: true
          });
        } else
          await chatStdout.write('(No response)');
      } else
        await chatStdout.write("\nai> "+responseText);
    }
    await chatStdout.write("\n\n");

    if (isPersistent)
      await fs.writeFile(currentChatHistoryFilePath + (sessionId ? '.' + sessionId : ''), JSON.stringify(chatHistory))
    if (chatHistory[channel] && isInitialization) {
      chatHistory[channel].length=0;
      chatHistory[channel].push(...JSON.parse(freezeHistory));
    }

    await clean(sessionId);
    return finalOutput;
  } catch (err) {
    await chatConsole.error(err); //"\nI'm sorry, I seem to having some issues at the moment.\n")
    return null;
  }
};
/*  const awaitedResponse = await streamResult.response;
  
  return awaitedResponse.candidates[0].content.parts[0].text;*/

export function setCurrentChatHistory(newCurrentChatHistoryFilePath) {
  currentChatHistoryFilePath = newCurrentChatHistoryFilePath;
}

export function verifyCurrentChatHistoryExists(sessionId = 0) {
  return _fs.existsSync(getCurrentChatHistoryFilename(sessionId));
}

export function getCurrentChatHistoryFilename(sessionId = 0) {
  return currentChatHistoryFilePath + (sessionId ? '.' + sessionId : '');
}

const initModel = async (sessionId) => {
    let project = setup.settings['computatron-ai-environment']
    if (!project) {
        if (!isInitialized[sessionId]) {
          defaultConsole.log("WARNING: No Computatron AI environment set. Defaulting to 'test'.")
          setup.settings['computatron-ai-project']='test';
        }
        project='test';
        isInitialized[sessionId]=true;
    }
    const sessionRoutesJSONFilePrefix = setup.settings['computatron-ai-session-routes-json-file-prefix'] || (project+"_");
    const port = setup.settings['computatron-ai-port'] || 3000;
    const host = setup.settings['computatron-ai-host'] || 'http://localhost';
    if (!project || old_project!=project) {
      old_project = project;
      generativeModel = await emulateVertexGenerativeModel(project, sessionId, sessionRoutesJSONFilePrefix+"session_routes.json", port, host);
    }
}

export async function init({
  action = 'Initializing',
  inputHistoryFilePath = null,
  outputHistoryFilePath = currentChatHistoryFilePath,
  initialChatInput = null,
  chatStdout = defaultStdout,
  chatConsole = defaultConsole,
  chatInstructions = defaultChatInstructions,
  newDefaultStdout,
  newDefaultConsole,
  newDefaultChatInstructions,
  sessionId = 0,
  isPersistent = true,
  isClearingMemory = true,
} = {}) {
  await initModel(sessionId);
  try {
    if (outputHistoryFilePath) {
      currentChatHistoryFilePath = outputHistoryFilePath;
    }
    if (newDefaultConsole) {
      defaultConsole = newDefaultConsole;
      chatConsole = defaultConsole;
    }
    if (newDefaultStdout) {
      defaultStdout = newDefaultStdout;
      chatStdout = defaultStdout;
    }
    if (newDefaultChatInstructions) {
      defaultChatInstructions = newDefaultChatInstructions;
      chatInstructions = defaultChatInstructions;
    }
    await chatConsole.log(`${action} chat with AI...`)
    let history = null;
    
    await clearMemory(defaultConsole, sessionId);
    
    if (!isClearingMemory && (inputHistoryFilePath || verifyCurrentChatHistoryExists(sessionId))) {
      const historyJSON = await fs.readFile(inputHistoryFilePath || currentChatHistoryFilePath, 'utf-8');
      history = JSON.parse(historyJSON);
      aiChatsMap[sessionId].historyInternal.length=0;
      aiChatsMap[sessionId].historyInternal.push(...history[0]);
    }
    let aiResponse = null;
    let isStartOfChat = (!aiChatsMap || !aiChatsMap[sessionId] || !aiChatsMap[sessionId].historyInternal || aiChatsMap[sessionId].historyInternal.length==0);
    //I hope you're well today
    if (isStartOfChat) {
      if (!chatInstructions && !initialChatInput) {
        aiResponse = await chat("Hi", {
          isInitialization: true,
          isPersistent: false,
          chatStdout: {
            write: () => {}
          },
          chatConsole,
          sessionId
        });
      } else {
        if (chatInstructions) {
          aiResponse = await chat(chatInstructions, {
            isPersistent,
            chatStdout: {
              write: () => {}
            },
            chatConsole,
            sessionId
          });
        }
        if (initialChatInput) {
          aiResponse = await chat(initialChatInput, {
            isPersistent,
            chatStdout,
            chatConsole,
            sessionId
          });
        }
      }
    }
    if (aiResponse || !isStartOfChat)
      isInitialized[sessionId] = true;
  } catch (err) {
    await defaultConsole.error(err);
  }
}

export const sleep = (delay) => new Promise((resolve) => setTimeout(resolve, delay))