import { getMjsFilePathsInDirectory } from 'fusion-hub/functions';
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
        log(command.examples)
        return
    }
    let readFilePath = await expandFilePath(input[0], {contextDir: 'fusion-hub/multi-command-history'})
    let commandText = await fs.readFile(readFilePath, 'utf-8')
    commandText = commandText.replace(/^[^\n]+\n/, 'multi-command ' + joinedInput + '\n')
    let hashCommandText = commandText.replace(/multi-command [^ ]+/, '')
    const hash = crypto.createHash('sha256').update(hashCommandText).digest('hex');


    let historyFilename = path.basename(input[0])
    if (/-[0-9a-f]{64}/.test(historyFilename))
    {
        historyFilename = historyFilename.replace(/-[^ ]{64}$/, '-' + hash)
    }
    else
    {
        historyFilename += hash
    }

    const historyFileShortPath = path.join('~~~', historyFilename)
    const historyFile = await expandFilePath(historyFileShortPath, {contextDir: 'fusion-hub/multi-command-history'})
    let historyExists = _fs.existsSync(historyFile)
    
    if (!historyExists)
    {
        await fs.writeFile(historyFile, commandText)
        
        log(`Creating history file as: ${historyFileShortPath}`)
    }
    else
    {
        log(`History file ${historyFile} already exists.`)
    }

    let headerVariables = commandText.split(/\r\n|\n/)
    let commands = commandText.split(new RegExp(headerVariables[1]))
    headerVariables = commands[0].split(/\r\n|\n/)
    const stopCommand = headerVariables[2]
    let scriptStart=3
    for (; scriptStart<headerVariables.length; scriptStart++)
    {
        if (headerVariables[scriptStart]=='script:')
            break;
    }

    let script={}
    if (headerVariables.length > scriptStart)
    {
        let codeToEval=headerVariables.slice(scriptStart+1).join('\n')

        eval(codeToEval)
    }

    for (let i=1; i<commands.length; i++)
    {
        if (commands[i] == stopCommand)
        {
            cliProperties.commandMode = null;
        }
        else 
        {
            try {
                await executeCommand(eval(`\`${commands[i]}\``), { isThrowing: true });
            } catch (err)
            {
                error(err + '\nAdditional information: failed on sub-command: ' + command[i]);
            }
        }
    }
}