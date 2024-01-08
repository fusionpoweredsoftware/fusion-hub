import { expandFilePath } from 'fusion-hub/functions';
import { executeCommand, error, properties as cliProperties } from 'fusion-hub/cli';
import fs from 'fs/promises'

export const command = {
    id: "multi-command",
    name: "multi-command",
    description: "Run multiple commands from a file.",
    examples: `multi-command <file.txt>`,
    isInputCaseSensitive: false,
    isStartable: false
};

export async function process({input}) {
    const filePath = await expandFilePath(input[0]);
    let commandText = await fs.readFile(filePath, 'utf-8');

    let headerVariables = commandText.split(/\r\n|\n/);
    let commands = commandText.split(new RegExp(headerVariables[0]));
    const stopCommand = headerVariables[1];

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