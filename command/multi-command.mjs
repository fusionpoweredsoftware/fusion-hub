import { expandFilePath } from 'fusion-hub/functions';
import { executeCommand, error, log, properties as cliProperties } from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';

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
        for (let i=1; i < codeToEvalSplit.length; i++)
        {
            let commandAndCode = codeToEvalSplit[i].split(new RegExp(lines[2]))
            if (commandAndCode.length == 2)
            {
                let escapedCommand = commandAndCode[0].replace(/`/g, '\\`')
                codeToEvalFinal += 'await evalCommand(`' + escapedCommand + '`, stopCommand);\n' + commandAndCode[1] + '\n'
            }
            else if (commandAndCode.length == 1)
            {
                let escapedCommand = commandAndCode[0].replace(/`/g, '\\`')
                codeToEvalFinal += 'await evalCommand(`' + escapedCommand + '`, stopCommand);\n'
            }
            else if (commandAndCode.length > 2)
            {
                log('Invalid syntax: Double terminator detected.');
            }
        }

        eval("(async () => {" + codeToEvalFinal + "})()" )
    }
}

async function evalCommand(command, stopCommand, varType)
{
    if (command == stopCommand)
    {
        cliProperties.commandMode = null;
    }
    else 
    {   
        try {
            let output = await executeCommand(eval(`\`${command.replace(/`/g, '\\`')}\``), { isThrowing: true });
            output = await filterOutput(output, {typeOfVar: 'JSON'});
        } catch (err)
        {
            error(err + '\nAdditional information:\n\nFailed on sub-command:\n' + command);
        }
    }

    return output
}

