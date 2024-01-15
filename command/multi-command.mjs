import { expandFilePath, extractLambdaFunctionBody, sleep, filterOutput, isObject, isString } from 'fusion-hub/functions';
import { error, log, properties as cliProperties, executeCommand } from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';
import json5 from 'json5'; // Are we using this?


const ai = {
    chatHistory: []
}
let lastCommand = null;
let lastStopCommand = null;
let lastVarType = null;
let lastOutput = null;

export const command = {
    id: "multi-command",
    name: "multi-command",
    description: "Run multiple commands from a file.",
    examples: `multi-command <file.txt> [parameters]`,
    isInputCaseSensitive: false,
    isStartable: false
  };

export async function process({input, joinedInput}) {
    if (input.length < 1 || input[0] == '')
    {
        log(command.examples);
        return;
    }
    let readFilePath = await expandFilePath(input[0], {contextDir: 'multi-command-history'})
    let commandText = await fs.readFile(readFilePath, 'utf-8');
    commandText = commandText.replace(/^[^\n]+\n/, 'multi-command ' + joinedInput + '\n');
    let hashCommandText = commandText.replace(/multi-command [^ ]+/, '');
    const hash = crypto.createHash('sha256').update(hashCommandText).digest('hex');


    let historyFilename = path.basename(input[0]);
    if (/-[0-9a-f]{64}/.test(historyFilename))
    {
        historyFilename = historyFilename.replace(/-[^ ]{64}$/, '-' + hash);
    }
    else
    {
        historyFilename += hash;
    }

    const historyFileShortPath = path.join('~~~', historyFilename);
    const historyFile = await expandFilePath(historyFileShortPath, {contextDir: 'multi-command-history'});
    let historyExists = _fs.existsSync(historyFile);
    
    if (!historyExists)
    {
        await fs.writeFile(historyFile, commandText);
        
        log(`Creating history file as: ${historyFileShortPath}`);
    }
    else
    {
        log(`History file ${historyFile} already exists.`);
    }

    let lines = commandText.split(/\r\n|\n/);
    const stopCommand = lines[3];
    let scriptStart=3;
    for (; scriptStart<lines.length; scriptStart++)
    {
        if (lines[scriptStart]=='script:')
            break;
    }
    let script={};
    if (lines.length > scriptStart)
    {
        let codeToEval= '\n' + lines.slice(scriptStart+1).join('\n');

        let codeToEvalSplit = codeToEval.split(new RegExp(lines[1]));
        let codeToEvalFinal = codeToEvalSplit[0];
        let commandBlockClosed = true; // are we using this?
        
        let commandAndCode = null
        for (let i=1; i < codeToEvalSplit.length; i++)
        {
            commandAndCode = codeToEvalSplit[i].split(new RegExp(lines[2]))

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
                const funcBodyToEval = async () => (await evalCommand(`${escapedCommand}`, { stopCommandIn: `${stopCommand}` , varTypeIn: `${varType}`, cliProperties}));
                const codeBody = extractLambdaFunctionBody(funcBodyToEval, { varType, escapedCommand, stopCommand });
                codeToEvalFinal += `${prependAssignment}${codeBody};\n`; 
                codeToEvalFinal += (commandAndCode.length == 2) ? commandAndCode[1] + '\n': '';
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

        log("(async () => {" + codeToEvalFinal + "})()")
        eval("(async () => {" + codeToEvalFinal + "})()" )
    }
}

export async function repeatCommand(additionalInfo)
{
    return await evalCommand(null, {repeatQuery: true, additionalInfo});
}

export async function evalCommand(commandIn, settings)
{
    if (typeof settings !== 'object' || settings === null) {
      error ("The evalCommand requires settings to be an object.");
      return;
    }
  
    settings ||= {};
  
    settings.stopCommandIn ||= null;
    settings.varTypeIn ||= null;
    settings.repeatQuery ||= false;

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
    }
    else
    {
        await sleep(5000);

        
        // contains kludge
        command = lastCommand + "  Please double check your JSON.  Your last response contained malformed JSON.";
        stopCommand = lastStopCommand;
        varType = lastVarType;
    }

    if (command == stopCommand)
    {
        cliProperties.commandMode = null;
    }
    else 
    {   
        try {
            const { chatHistory } = ai;
            
            output = await executeCommand(eval(`\`${command.replace(/`/g, '\\`')}${(repeatQuery && isString(lastOutput)) ? `\n\nUse the output from the JSON parser to help resolve the issue: ${lastOutput}` : ''} ${additionalInfo||''}\``), { isThrowing: true, aiChatSettings: {isStreaming: false, chatHistory }  });
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
                output = await executeCommand(eval(`\`${command.replace(/`/g, '\\`')}\nDo not leave any of the JSON out.  You must produce valid JSON.  Do not produce the same response as last time!!! ${isErrorMessage ? `\n\nUse the output from the JSON parser to help resolve the issue: ${output}` : ''}\``), { isThrowing: true, aiChatSettings: {isStreaming: false, chatHistory}  });if (varType && output)
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
