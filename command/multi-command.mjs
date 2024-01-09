import { getMjsFilePathsInDirectory } from 'fusion-hub/functions';
import { expandFilePath } from 'fusion-hub/functions';
import { executeCommand, error, properties as cliProperties } from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises'

export const command = {
    id: "multi-command",
    name: "multi-command",
    description: "Run multiple commands from a file.",
    examples: `multi-command <file.txt>`,
    isInputCaseSensitive: false,
    isStartable: false
  };

export async function process({input, joinedInput}) {
    let commandText = await fs.readFile(input[0], 'utf-8')
    const historyFile = await expandFilePath(path.join('~~~', input[0] + `-${new Date()
        .toISOString()
        .replace(/:/g, "-")
        .slice(0, 19)}`), {contextDir: 'multi-command-history'})

    await fs.writeFile(historyFile, commandText.replace(/^[^\n]+\n/, joinedInput))

    let headerVariables = commandText.split(/\r\n|\n/)
    let commands = commandText.split(new RegExp(headerVariables[1]))
    const stopCommand = headerVariables[2]

    for (let i=1; i<commands.length; i++)
    {
        if (commands[i] == stopCommand)
        {
            cliProperties.commandMode = null;
        }
        else 
        {
            try {
                await executeCommand(commands[i], { isThrowing: true });
            } catch (err)
            {
                error(err + '\nAdditional information: failed on sub-command: ' + command[i]);
            }
        }
    }
}