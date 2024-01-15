import { expandFilePath, filterOutput, sleep } from 'fusion-hub/functions';
import { executeCommand, error, log, properties as cliProperties } from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';
import json5 from 'json5'



let lastCommand = null;
let lastStopCommand = null;
let lastVarType = null;

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
        let commandBlockClosed = true;
        
        let commandAndCode = null
        for (let i=1; i < codeToEvalSplit.length; i++)
        {
            commandAndCode = codeToEvalSplit[i].split(new RegExp(lines[2]))

            if (commandAndCode.length >= 1 && commandAndCode.length <= 2)
            {
                let prependAssignement = '';
                let varType = 'null';
                let matches = commandAndCode[0].match(/^\([ ]*(?<varName>[_a-zA-Z0-9]+)[ ]*,[ ]*(?<varType>[_a-zA-Z0-9]+)[ ]*\)\n?/m);
                if (matches != null)
                {
                    prependAssignement = matches.groups.varName ? ('let ' + matches.groups.varName + '=') : null;
                    varType = matches.groups.varType || null;
                
                    if (prependAssignement && varType && (varType == 'JSON' || varType == 'text'))
                    {
                        commandAndCode[0] = commandAndCode[0].replace(/^\([ ]*(?<varName>[_a-zA-Z0-9]+)[ ]*,[ ]*(?<varType>[_a-zA-Z0-9]+)[ ]*\)\n?/m, '')
                    }
                    else
                    {
                        error('Invalid command assignment: (' + matches.groups.varName + ', ' + varType + ')');
                    }
                }
                let escapedCommand = commandAndCode[0].replace(/`/g, '\\`')
                codeToEvalFinal += prependAssignement + 'await evalCommand(`' + escapedCommand + '`, stopCommand, `' + varType + '`);\n' 
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


async function repeatCommand()
{
    return await evalCommand(null, null, null, true);
}

async function evalCommand(commandIn, stopCommandIn, varTypeIn, repeatQuery=false)
{
    let output = null;

    let command = commandIn;
    let stopCommand = stopCommandIn;
    let varType = varTypeIn;

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
        command = lastCommand + "  Please double check your JSON5.  Your last response contained malformed JSON5.";
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
            output = await executeCommand(eval(`\`${command.replace(/`/g, '\\`')}\``), { isThrowing: true, aiChatSettings: {isStreaming: false} });
            if (varType!='null' && output!=null && output != undefined)
            {
                output = await filterOutput(output, {typeOfVar: varType});
            }
            /// kludge
            if ((output==undefined || output==null) && varType!='null')
            {
                output = await executeCommand(eval(`\`${command.replace(/`/g, '\\`')}\nDo not leave any of the JSON5 out.  You must produce valid JSON5.  Do not produce the same response as last time!!!\``), { isThrowing: true, aiChatSettings: {isStreaming: false}  });
                if (varType && output)
                {
                    output = await filterOutput(output || null, {typeOfVar: varType});
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

