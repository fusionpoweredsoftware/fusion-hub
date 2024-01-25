import os from "os";
import fs from "fs/promises";
import _fs from "fs";
import path, { parse } from "path";
import { cliRoot } from "fusion-hub/paths";
import setup from "fusion-hub/setup";
import json5 from "json5";
let codeBlockCounter = 0;

//If you have some troublesome JSON outputs, put Tricky in the front and specify
//specific keys that you are only interested in.
//By default, if you specify no keys then it will attempt to parse normally and return the error as a string if it failed.

export function parseTrickyJSON(inputString, keys) {
  try {
      // First, try parsing as standard JSON
      return json5.parse(inputString);
  } catch (err) {
      if (keys.length==0) {
        return `Malformed JSON: ${err}`;
      }
      // If standard JSON parsing fails, use the custom parser
      const result = {};
      let lastIndex = 0;

      keys.forEach(key => {
          inputString=inputString.replace(new RegExp(`(")?${key}(")?( )*:`,"g"),key+":");
          inputString=inputString.replace(new RegExp(`(')?${key}(')?( )*:`,"g"),key+":");
          inputString=inputString.replace(new RegExp(`(\`)?${key}(\`)?( )*:`,"g"),key+":");
          inputString=inputString.replace(new RegExp(`[\n,{]*( )*${key}:`,"g"),","+key+":");
      });
      keys.forEach(key => {
          const keyIndex = inputString.indexOf(","+key + ":", lastIndex);
          if (keyIndex !== -1) {
              // Find the end index for the current key's value
              let endIndex = inputString.length;
              for (let nextKey of keys) {
                  if (nextKey !== key) {
                      const nextKeyIndex = inputString.indexOf(+","+nextKey + ":", keyIndex + key.length);
                      if (nextKeyIndex !== -1 && nextKeyIndex < endIndex) {
                          endIndex = nextKeyIndex;
                      }
                  }
              }

              // Extract and assign the value for the current key
              const value = inputString.substring(keyIndex + key.length + 1, endIndex).trim();
              result[key] = value;
              lastIndex = keyIndex + key.length;
          }
      });

      return result;
  }
}

export function extractLambdaFunctionBody(func, substitutions) {
  const functionString = func.toString().trim();
  let body = null;

  if (functionString.startsWith("(") || functionString.startsWith("async")) {
    body = functionString
      .match(/^(?: async [(])?[^>]*[)](?: )*=>(?: )*[(]?(?: )*([\s\S]*)(?: )*[)](?: )*?/)[1]
      .trim();
  }
  if (!body)
    throw "Not a lambda function!";

  // Replace variable placeholders with substitutions
  for (const [variableName, replacement] of Object.entries(substitutions)) {
    const regex = new RegExp(`\\$\\{${variableName}\\}`, "g");
    body = body.replace(regex, replacement);
  }
  body = body.replace(/\`null\`/g, 'null');
  return body;
}

export function extractJson5Objects(inputString, settings) {
  const { isEnglishOnly } = (settings ||= {});
  const jsonObjects = [];
  const lines = inputString.split('\n');
  let isInsideJsonBlock = false;
  let currentJsonBlock = '';

  for (let l = 0; l < lines.length; l++) {
    let line = lines[l];
    if (line.trim().toLowerCase().match(/```( )*(json)?/g) && lines[l+1] && /[\{\[]/g.test(lines[l+1].trim())) {
      isInsideJsonBlock = true;
      currentJsonBlock = '';
    } else if (isInsideJsonBlock && line.trim() === '```') {
      isInsideJsonBlock = false;
      try {
        let nonEnglishStrings = extractContinuousJapaneseAndChinese(currentJsonBlock);
        if (!isEnglishOnly || nonEnglishStrings.length==0) {
          let parsedObject;
          if (settings.isTrickyJSON) {
            parsedObject=parseTrickyJSON(currentJsonBlock,(settings.varTypeSettings||'').split(","));
          } else {
            parsedObject=json5.parse(currentJsonBlock);
          }
          jsonObjects.push(parsedObject);
        } else {
          throw `Non-english Characters Detected: ${nonEnglishStrings}`;
        }
        currentJsonBlock = ''; // Reset the current block
      } catch (err) {
        jsonObjects.push(`Error parsing JSON: ${err}\n\nJSON parsed: ${currentJsonBlock}`);
      }
    } else if (isInsideJsonBlock) {
      currentJsonBlock += line + '\n';
    }
  }
  
  if (jsonObjects.length==0) {
    try {
      return [parseTrickyJSON(inputString,(settings.varTypeSettings||'').split(","))];
    } catch (err) {
      return ["JSON was malformed."]
    }
  }

  return jsonObjects;
}

export const filterOutput = (output, filterSettings=null) => {
  if (output == null)
    return output
  const { varTypeSettings } = filterSettings;
  if (filterSettings.typeOfVar == 'TrickyJSON')
  {
    const json = extractJson5Objects(output, { isEnglishOnly: true, isTrickyJSON: true, varTypeSettings});
    return (json.length > 0) ? json[0] : null;
  } 
  else if (filterSettings.typeOfVar == 'JSON')
  {
    const json = extractJson5Objects(output, { isEnglishOnly: true });
    return (json.length > 0) ? json[0] : null;
  } 
  else if (filterSettings.typeOfVar == 'JSONNonEnglish') {
    const json = extractJson5Objects(output, { isEnglishOnly: false });
    return (json.length > 0) ? json[0] : null;
  } else if (filterSettings.typeOfVar == "text") {
    return output;
  }

  return null;
};

export const getArgs = (input) => {
  input = input.trim().match(/(?:[^\s']+|'[^'\\]*(?:\\.[^'\\]*)*')+/g);
  if (!input)
    return [''];
  for (let i = 0; i < input.length; i++)
    if (input[i][0] == `'` && input[i].match(/'$/g))
      input[i] = input[i].slice(1, -1).replace("\\'", "'");
  return input;
};

export const expandFilePath = async (
  filePath,
  { appCacheDir, contextDir } = {}
) => {
  const { currentAppCacheDir, defaultContextDir } = await import(
    "fusion-hub/paths"
  );
  const cli = await import(
    "fusion-hub/cli"
  );
  contextDir ||= defaultContextDir;
  appCacheDir ||= currentAppCacheDir;
  if (/[a-z]/ig.test(filePath[0])) {
    filePath = path.join("~~~",filePath);
  }
  const fullFilePath = filePath
    .replace(/^\~\~\~/g, path.join(os.homedir(), "." + appCacheDir, contextDir))
    .replace(/^\~\~/g, path.join(os.homedir(), "." + appCacheDir))
    .replace(/^\~/g, os.homedir())
    .replace(/^\.\//g, cliRoot+'/');
  if (!_fs.existsSync(fullFilePath)) {
    await fs.mkdir(path.dirname(fullFilePath), { recursive: true });
  }
  return fullFilePath;
};

export const generateUniqueId = () => {
  const timestamp = new Date().getTime().toString(16);
  const randomPart = Math.random().toString(16).substring(2);
  return `${timestamp}-${randomPart}`;
};

export async function getMjsFilePathsInDirectory(dirPath) {
  const cli = await import("fusion-hub/cli");
  const mjList = [];
  try {
    const files = await fs.readdir(dirPath);
    for (const file of files) {
      const filePath = path.join(dirPath, file);
      if (path.extname(filePath) === ".mjs") {
        mjList.push(filePath);
      }
    }
  } catch (error) {
    cli.error(`Error reading directory: ${error}`);
  }
  return mjList;
}

export function extractJSONFilename(dataString) {
  // Regular expression to match a JSON filename within double quotes or without quotes
  const regex = /"([^"]+\.json)"|(\S+\.json)/;

  // Match the regex in the input dataString
  const match = dataString.match(regex);

  if (match) {
    // The JSON filename can be in either group 1 or group 2 of the match
    return (match[1] || match[2]).replace(/["'`]/g, "");
  } else {
    // If no match is found, return null or handle the case as needed
    return null;
  }
}
export function isLineHitNTimesInMS(n, m) {
  if (codeBlockCounter >= n - 1) {
    return true; // Condition met
  }

  codeBlockCounter++;

  setTimeout(() => {
    codeBlockCounter = 0; // Reset the counter
  }, m);

  return false; // Condition not met
}

export function capitalizeFirstLetter(string) {
  return string.charAt(0).toUpperCase() + string.slice(1);
}

export const responseWriteJSON = (res, jsonResponseChunk) => {
  res.write("\n"); // Separating chunks with newline for JSON streaming
  res.write(JSON.stringify(jsonResponseChunk));
};

export const getChatStdout = (res, { sessionId, responseId }, response) => {
  return {
    write: (message) =>
      response(res, {
        sessionId,
        responseId,
        message,
      }),
  };
};

export const pullCliHistory = async (rl, commandMode) => {
  const { getCommandModeHistoryFilePath, getCliHistoryFilePath } = await import(
    "fusion-hub/paths"
  );
  if (commandMode) {
    const commandModeFileHistoryPath = await getCommandModeHistoryFilePath();
    if (_fs.existsSync(commandModeFileHistoryPath))
      rl.history = (await fs.readFile(commandModeFileHistoryPath, "utf-8"))
        .split("\n")
        .filter((line) => line.trim() !== "");
    else rl.history = [];
  } else {
    const cliHistoryFilePath = await await getCliHistoryFilePath();
    if (_fs.existsSync(cliHistoryFilePath))
      rl.history = (await fs.readFile(cliHistoryFilePath, "utf-8"))
        .split("\n")
        .filter((line) => line.trim() !== "");
    else rl.history = [];
  }
};

export const pushCliHistory = async (rl, commandMode) => {
  const { getCommandModeHistoryFilePath, getCliHistoryFilePath } = await import(
    "fusion-hub/paths"
  );
  if (commandMode) {
    await fs.writeFile(
      await getCommandModeHistoryFilePath(),
      rl.history.filter((line) => line.trim() !== "").join("\n")
    );
  } else {
    await fs.writeFile(
      await getCliHistoryFilePath(),
      rl.history.filter((line) => line.trim() !== "").join("\n")
    );
  }
};

export const pullSetupSettings = async () => {
  const { getSetupSettingsFilePath } = await import("fusion-hub/paths");
  const setupSettingsFilePath = await getSetupSettingsFilePath();
  let fileText = "",
    settings = {};
  if (_fs.existsSync(setupSettingsFilePath)) {
    fileText = await fs.readFile(setupSettingsFilePath, "utf-8");
    settings = JSON.parse(fileText);
  }
  Object.assign(setup.settings, settings);
};

export const pushSetupSettings = async () => {
  const { getSetupSettingsFilePath } = await import("fusion-hub/paths");
  const setupSettingsFilePath = await getSetupSettingsFilePath();
  const settings = setup.settings || {};
  const fileText = JSON.stringify(settings);
  await fs.writeFile(setupSettingsFilePath, fileText);
};

export function getTimestamp() {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  const hours = String(now.getHours()).padStart(2, "0");
  const minutes = String(now.getMinutes()).padStart(2, "0");
  const seconds = String(now.getSeconds()).padStart(2, "0");
  const milliseconds = String(now.getMilliseconds()).padStart(3, "0");

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}.${milliseconds}`;
}

export async function stabilizeOutput({repeatCommand, jsonOutput, jsonComponent,targetJson,targetJsonComponent=jsonComponent,attempt=0, attemptMax=5}) {
  while ((!isObject(jsonOutput) || !jsonOutput[jsonComponent] || !isIterable(jsonOutput[jsonComponent]) || jsonOutput[jsonComponent].length == 0) && attempt++<attemptMax) {
    jsonOutput = await repeatCommand();
  }
  
  if (jsonOutput[jsonComponent] && jsonOutput[jsonComponent].length>0) {
    targetJson[targetJsonComponent] = jsonOutput[jsonComponent];
  } else    
    targetJson[targetJsonComponent] = [];
  return jsonOutput;
}

export const addPeriodToBasename = (filePath) => path.join(path.dirname(filePath), `.${path.basename(filePath)}`);
export const sleep = (delay) => new Promise((resolve) => setTimeout(resolve, delay))
export const isObject = value => value !== null && typeof value === 'object' && !Array.isArray(value);
export const isString = value => typeof value === 'string';

export const isIterable = (obj) =>
{
    // checks for null and undefined
    if (obj == null) {
        return false;
    }
    return typeof obj[Symbol.iterator] === 'function';
}

function extractContinuousJapaneseAndChinese(inputString) {
  const regex = /[\u4e00-\u9fa5\u3040-\u309F\u30A0-\u30FF]+/g;
  const matches = inputString.match(regex);
  
  if (matches) {
    return matches.map(match => `${match}`);
  } else {
    return [];
  }
}