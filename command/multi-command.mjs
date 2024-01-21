import { expandFilePath, extractLambdaFunctionBody, filterOutput, isObject, isString, stabilizeOutput, addPeriodToBasename } from 'fusion-hub/functions';
import cli, { properties as cliProperties, executeCommand } from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';
import json5 from 'json5';

export const ai = {
    chatHistory: []
}

export const parser = {
    json5,
    stabilizeOutput
}

export const lastSetup = {};

let lastCommand = null;
let lastStopCommand = null;
let lastVarType = null;
let lastOutput = null;
let lastSettings = null;

export const command = {
    id: "multi-command",
    name: "multi-command",
    description: "Run multiple commands from a file.",
    examples: `multi-command <file.txt> [parameters]`,
    isInputCaseSensitive: true,
    isStartable: false
  };

async function reloadModule(modulePath) {
    // Append a unique timestamp as a query string
    const newPath = `${modulePath}?update=${Date.now()}`;

    // Dynamically import the module with the new path
    const module = await import(newPath);

    return module;
}

export async function process({input, joinedInput, settings = {}}) {
    const { error, log } = cli.output;
    const filePath = input[0];
    if (input.length < 1 || filePath == '')
    {
        log(command.examples);
        return;
    }
    settings.aiChatSettings ||= {};
    settings.app ||= {};
    settings.app.input ||= '';
    settings.app.settings ||= {};
    const { mcFileContextDir = 'multi-command', historyContextDir = 'multi-command-history'  } = settings;
    const { isLoggingMultiCommandFileContents = true, isVerbose = true, isInterruptable = true, app = {}, aiChatSettings = {}} = settings;

    let readFilePath = await expandFilePath(filePath, {contextDir: mcFileContextDir})
    let commandText = await fs.readFile(readFilePath, 'utf-8');
    commandText = commandText.replace(/^[^\n]+\n/, 'multi-command ' + joinedInput + '\n');
    let hashCommandText = commandText.replace(/multi-command [^ ]+/, '');
    const hash = crypto.createHash('sha256').update(hashCommandText).digest('hex');

    let historyFilename = path.basename(filePath);
    if (/-[0-9a-f]{64}/.test(historyFilename))
    {
        historyFilename = historyFilename.replace(/-[^ ]{64}$/, '-' + hash);
    }
    else
    {
        historyFilename += hash;
    }

    const historyFileShortPath = path.join('~~~', historyFilename);
    const historyFile = await expandFilePath(historyFileShortPath, {contextDir: historyContextDir});
    let historyExists = _fs.existsSync(historyFile);
    
    if (!historyExists)
    {
        await fs.writeFile(historyFile, commandText);
        if (isVerbose)
            log(`Creating history file as: ${historyFileShortPath}`);
    }
    else
    {
        if (isVerbose)
            log(`History file ${historyFile} already exists.`);
    }

    let lines = commandText.split(/\r\n|\n/);
    const stopCommand = lines[3].slice(2);
    let scriptStart=3;
    for (; scriptStart<lines.length; scriptStart++)
    {
        if (lines[scriptStart].slice(2)=='script:')
            break;
    }
    let script={};
    if (lines.length > scriptStart)
    {
        let codeToEval= '\n' + lines.slice(scriptStart+1).join('\n');

        let codeToEvalSplit = codeToEval.split(new RegExp(lines[1].slice(2)));
        let codeToEvalFinal = codeToEvalSplit[0]+'\n';
        let commandBlockClosed = true; 
        Object.assign(lastSetup, {
            scriptVariables: script,
            commandBlockClosed
        });
        
        let commandAndCode = null
        for (let i=1; i < codeToEvalSplit.length; i++)
        {
            commandAndCode = codeToEvalSplit[i].split(new RegExp(lines[2].slice(2)))

            if (commandAndCode.length >= 1 && commandAndCode.length <= 2)
            {
                let prependAssignment = '';
                let varType = 'null';
                let matches = commandAndCode[0].match(/^\([ ]*(?<varName>[_a-zA-Z0-9]+)[ ]*,[ ]*(?<varType>[_a-zA-Z0-9]+)[ ]*\)\n?/m);
                if (matches != null)
                {
                    prependAssignment = matches.groups.varName ? ('let ' + matches.groups.varName + '=') : null;
                    varType = matches.groups.varType || null;
                
                    if (prependAssignment && varType && (varType == 'JSON' || varType == 'text'))
                    {
                        commandAndCode[0] = commandAndCode[0].replace(/^\([ ]*(?<varName>[_a-zA-Z0-9]+)[ ]*,[ ]*(?<varType>[_a-zA-Z0-9]+)[ ]*\)\n?/m, '')
                    }
                    else
                    {
                        error('Invalid command assignment: (' + matches.groups.varName + ', ' + varType + ')');
                    }
                }
                const escapedCommand = commandAndCode[0].replace(/`/g, '\\`')
                const funcBodyToEval = async () => (await evalCommand(`${escapedCommand}`, { aiChatSettings, stopCommandIn: `${stopCommand}` , varTypeIn: `${varType}`, cliProperties}));
                const codeBody = extractLambdaFunctionBody(funcBodyToEval, { varType, escapedCommand, stopCommand });
                codeToEvalFinal += `\n${prependAssignment}${codeBody};\n`; 
                codeToEvalFinal += (commandAndCode.length == 2) ? commandAndCode[1]: '';
            }
            else if (commandAndCode.length > 2)
            {
                error('Invalid syntax: Double terminator detected.');
            }
        }

        if (commandAndCode && commandAndCode.length != 2)
        {
            error('Open command without close.');
        }
        const mcRunner = await fs.readFile('./mc-runner.mjs','utf-8');
        codeToEvalFinal = mcRunner.replace("/*${codeToEvalFinal}*/",codeToEvalFinal);
        let debuggableFile = addPeriodToBasename(readFilePath)
        debuggableFile = `${debuggableFile}.mjs`.replace(/\.js\.mjs$/g,'.mjs');
        if (isLoggingMultiCommandFileContents)
            log(codeToEvalFinal)
        await fs.writeFile(debuggableFile,codeToEvalFinal);
        const importedCode = await reloadModule(debuggableFile);
        if (isInterruptable) {
            importedCode.run(input, app, aiChatSettings);
        } else
            await importedCode.run(input, app, aiChatSettings);
    }
}

export async function repeatCommand(additionalInfo)
{
    return await evalCommand(null, { repeatQuery: true, additionalInfo});
}

export async function evalCommand(commandIn, settings)
{
    const { error } = cli.output;
    if (typeof settings !== 'object' || settings === null) {
      error ("The evalCommand requires settings to be an object.");
      return;
    }
  
    settings ||= {};
  
    settings.stopCommandIn ||= null;
    settings.varTypeIn ||= null;
    settings.repeatQuery ||= false;
    settings.aiChatSettings ||= {};
    
    const { repeatQuery } = settings;
    let output = null;

    let command = commandIn;
    let { stopCommandIn: stopCommand, varTypeIn: varType, additionalInfo } = settings;

    if (additionalInfo)
        additionalInfo+=" ";

    if (!repeatQuery)
    {
        lastCommand = command;
        lastStopCommand = stopCommand;
        lastVarType = varType;
        lastSettings = settings;
    }
    else
    {
        command = lastCommand;
        stopCommand = lastStopCommand;
        varType = lastVarType;
        settings = lastSettings;
        if (varType=='JSON')
            command += "  Please double check your JSON.  Your last response contained malformed JSON.";
    }

    const { isCommandStartLogged = false, aiChatSettings = {} } = settings;

    Object.assign(aiChatSettings, settings.aiChatSettings);
    aiChatSettings.isStreaming ||= false;
    aiChatSettings.chatHistory = ai.chatHistory;

    if (command == stopCommand)
    {
        cliProperties.commandMode = null;
    }
    else 
    {   
        try {
            const finalCommand = await eval(`(async    => \`${command.replace(/`/g, '\\`')}${(repeatQuery && isString(lastOutput) && varType=='JSON') ? `\n\nUse the output from the JSON parser to help resolve the issue: ${lastOutput}` : ''} ${additionalInfo||''}\`)()`);
            output = await executeCommand(finalCommand, { isThrowing: true, aiChatSettings, isCommandStartLogged  });
            if (varType!='null' && isString(output))
            {
                output = await filterOutput(output, {typeOfVar: varType});
            }
            if (!repeatQuery)
                lastOutput=output;
            /// kludge
            const isErrorMessage = isString(output);
            if ((!isObject(output) || isErrorMessage) && varType=='JSON')
            {
                output = await executeCommand(await eval(`(async () => \`${command.replace(/`/g, '\\`')}\nDo not leave any of the JSON out.  You must produce valid JSON.  Do not produce the same response as last time!!! ${isErrorMessage ? `\n\nUse the output from the JSON parser to help resolve the issue: ${output}` : ''}\`)()`), { isThrowing: true, aiChatSettings, isCommandStartLogged  });
                if (varType && output)
                {
                    output = await filterOutput(output, {typeOfVar: varType});
                }
            }
        } catch (err)
        {
            output = null;
            error(err + '\nAdditional information:\n\nFailed on sub-command:\n' + command);
        }
    }

    return output;
}
