import readline from 'readline';

import {
  defaultCommandPath,
  commandModuleRoot
} from 'fusion-hub/paths';

import {
  isLineHitNTimesInMS,
  pullCliHistory,
  pushCliHistory,
  getTimestamp,
  getArgs
} from 'fusion-hub/functions';

export const properties = {
  isPromptRunning: true,
  commandMode: null,
  lastCommandMode: null,
  lockModeTo: null,
  commandPath: null,
  commandModuleRoot: null
};

const event = (eventType, eventData, stamp=`[${getTimestamp()}]`, seperator = ' - ') => {
  return eventData ? console[eventType](stamp + seperator + eventData) : console[eventType](stamp);
}

export const log = (eventData, stamp, seperator) => event('log', eventData, stamp, seperator);
export const error =  (eventData, stamp, seperator) => event('error', eventData, stamp, seperator);
export const warn =  (eventData, stamp, seperator) => event('warn', eventData, stamp, seperator);
export let write = (data) => process.stdout.write(data);
export const print = (data) => write((data||='')+'\n');

let rl;

export const executeCommand = async (input, settings = {}, extensiveInputBuffer = null) => {
  if (!input || !input.split)
    return;
  input = getArgs(input);
  if (input.length > 0 && input[0].length > 0) {
    const command = properties.commandMode ? properties.commandMode : input[0].toLowerCase();
    if (properties.lockModeTo && properties.lockModeTo != command)
      return;
    if (!properties.commandMode)
      input = input.slice(1);
    let joinedInput = input.join(" "), lowercaseJoinedInput = joinedInput.toLowerCase(), lowercaseInput = lowercaseJoinedInput.split(" ");
    try {
      properties.commandPath ||= defaultCommandPath;
      properties.commandModuleRoot ||= commandModuleRoot;
      const commandModulePath = properties.commandModuleRoot + '/' + command + ((properties.commandModuleRoot[0].match(/[\.\/]/g)) ? ".mjs" : '');
      const importedCommand = await import(commandModulePath);
      if (!importedCommand.command.isInputCaseSensitive) {
        joinedInput = lowercaseJoinedInput;
        input = joinedInput.split(" ");
      }
      const joinedCommaDelimitedInput = joinedInput.split(","), commaDelimitedInput = [];
      for (let i = 0; i < commaDelimitedInput.length; i++) {
        commaDelimitedInput.push(commaDelimitedInput[i].split(" "));
      }
      await importedCommand.process({
        input,
        joinedInput,
        lowercaseJoinedInput,
        lowercaseInput,
        settings,
        extensiveInputBuffer,
        joinedCommaDelimitedInput,
        commaDelimitedInput
      });
    } catch (err) {
      if (settings.isThrowing)
      {
        throw err;
      }
      else
      {
        if (err.code != 'ERR_MODULE_NOT_FOUND')
          error(err);
        else
          commandNotFoundError();
      }
    }
  }
}

async function init(initialInput) {
  rl = readline.createInterface({
    input: process.stdin,
    output: process.stdout
  });
  await pullCliHistory(rl, properties.commandMode);
  if (initialInput) {
    rl.pause();
    await executeCommand(initialInput);
  }
  
  print();

  rl.setPrompt((properties.commandMode || '') + '> ');
  rl.prompt();

  rl.on('line', async (input) => {
    const commandModeBefore = properties.commandMode;
    try {
      rl.pause();
      await executeCommand(input);
    } catch (err) {
      error(err);
    }
    rl.setPrompt((properties.commandMode || '') + '> ');
    if (properties.isPromptRunning) {
      if (commandModeBefore!=properties.commandMode) {
        await pushCliHistory(rl, !properties.commandMode);
        await pullCliHistory(rl, properties.commandMode);
      }
      rl.prompt()
    } else
      rl.close();
  }).on('close', async () => {
    await pushCliHistory(rl, properties.commandMode);
    await pullCliHistory(rl, !properties.commandMode);
    if (!properties || !properties.isPromptRunning)
      process.exit();
    if (properties.lockModeTo || isLineHitNTimesInMS(2, 350))
      process.exit(0)
    if (properties.commandMode) {
      properties.lastCommandMode = properties.commandMode;
      properties.commandMode = null;
      await init();
    } else {
      properties.commandMode = properties.lastCommandMode || '';
      rl.close();
      await init();
    }
  });
}
const commandNotFoundError = () => error("Command not found.");
const noCommandSuppliedError = () => error("Unable to process command: No command supplied.");
const invalidStartTypeError = () => error("Unable to process command: Not a valid start type.");
const commandNotStartableError = () => error("Unable to process command: Not a valid command mode to start.");
const noArgumentsAllowedError = () => error('Command takes no arguments.');
const templateNotFoundError = () => error('Template not found');
const noArgumentsSuppliedError = (minimum) => error(`Command takes ${minimum} argument${(minimum>1)?'s':''}.`);
const argumentsLimitError = (limit) => error(`Command limited to ${limit} argument${(limit>1)?'s':''}.`);

export default {
  properties,
  init,
  errors: {
    noArgumentsAllowedError,
    noArgumentsSuppliedError,
    argumentsLimitError,
    commandNotStartableError,
    invalidStartTypeError,
    noCommandSuppliedError,
    commandNotFoundError,
    templateNotFoundError,
  },
  output: {
    log,
    error,
    warn,
    write,
    print,
  }
}
