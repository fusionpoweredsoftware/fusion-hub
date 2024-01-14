import os from "os";
import fs from "fs/promises";
import _fs from "fs";
import path from "path";
import setup from "fusion-hub/setup";
import json5 from "json5";
let codeBlockCounter = 0;

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

  return body;
}

export function extractJson5Objects(text) {
  let braceCount = 0;
  let startIndex = 0;
  let jsonObjects = [];

  for (let i = 0; i < text.length; i++) {
      if (text[i] === '{') {
          braceCount += 1;
          if (braceCount === 1) {
              startIndex = i; // Mark the start of a JSON5 object
          }
      } else if (text[i] === '}') {
          braceCount -= 1;
          if (braceCount === 0) {
              // End of a JSON5 object
              try {
                  const jsonObj = json5.parse(text.substring(startIndex, i + 1));
                  jsonObjects.push(jsonObj);
              } catch (e) {
                  jsonObjects = [] // Ignore if the string is not a valid JSON5 object
              }
          }
      }
    }
  }

  if (jsonObjects.length === 0)
  {
    return null;
  }
  
  // Return based on the number of valid JSON5 objects
  return jsonObjects.length === 1 ? jsonObjects[0] : jsonObjects;
}

export const filterOutput = (output, filterSettings=null) => {
  if (output == null)
    return output

  if (filterSettings.typeOfVar == 'JSON')
  {
    return extractJson5Objects(output);
  } else if (filterSettings.typeOfVar == "text") {
    return output;
  }

  return null;
};

export const getArgs = (input) => {
  input = input.trim().match(/(?:[^\s']+|'[^'\\]*(?:\\.[^'\\]*)*')+/g);
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
  contextDir ||= defaultContextDir;
  appCacheDir ||= currentAppCacheDir;
  const fullFilePath = filePath
    .replace(/^\~\~\~/g, path.join(os.homedir(), "." + appCacheDir, contextDir))
    .replace(/^\~\~/g, path.join(os.homedir(), "." + appCacheDir))
    .replace(/^\~/g, os.homedir());
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

export const addPeriodToBasename = (filePath) => path.join(path.dirname(filePath), `.${path.basename(filePath)}`);
export const sleep = (delay) => new Promise((resolve) => setTimeout(resolve, delay))
