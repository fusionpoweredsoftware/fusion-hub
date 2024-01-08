import { getMjsFilePathsInDirectory } from 'fusion-hub/functions';
import cli, {executeCommand, properties, error} from 'fusion-hub/cli';
import path from 'path';
import fs from 'fs/promises'
const { properties: cliProperties } = cli;

export const command = {
    id: "multi-command",
    name: "multi-command",
    description: "Run multiple commands from a file.",
    examples: `multi-command <file.txt>`,
    isInputCaseSensitive: false,
    isStartable: false
  };

// export async function listCommandModules({ commandPath } = cliProperties) {
//     const commandModuleFilePaths = await getMjsFilePathsInDirectory(commandPath);
//     const col = [13, 75, 34];
//     const c =  "--|--", t =  "  |  ";
//     cli.output.print();
//     cli.output.print('COMMAND'.padEnd(col[0],' ') + t + 'DESCRIPTION'.padEnd(col[1],' ') + t + 'EXAMPLES'.padEnd(col[2],' '));
//     cli.output.print(''.padEnd(col[0],'-') + c + ''.padEnd(col[1],'-') + c + ''.padEnd(col[2],'-'));
//     cli.output.print(''.padEnd(col[0],' ') + t + ''.padEnd(col[1],' ') + t + ''.padEnd(col[2],' '));
//     for (let commandModuleFilePath of commandModuleFilePaths) {
//         const commandModuleName = cliProperties.commandModuleRoot+"/"+path.basename(commandModuleFilePath.replace(/\.mjs$/g,''));
//         const commandModule = await import(commandModuleName);
//         cli.output.print(commandModule.command.id.padEnd(col[0],' ') + t + commandModule.command.description.padEnd(col[1],' ') + "  |  " + commandModule.command.examples.padEnd(col[2],' '));
//         cli.output.print(''.padEnd(col[0],' ') + t + ''.padEnd(col[1],' ') + t + ''.padEnd(col[2],' '));
//     }
// }

export async function process({input}) {
    let commandText = await fs.readFile(input[0], 'utf-8')

    let headerVariables = commandText.split(/\r\n|\n/)
    let commands = commandText.split(new RegExp(headerVariables[0]))
    const stopCommand = headerVariables[1]

    for (let i=1; i<commands.length; i++)
    {
        if (commands[i] == stopCommand)
        {
            properties.commandMode = null
        }
        else 
        {
            try {
                await executeCommand(commands[i], undefined, undefined, true)
            } catch (err)
            {
                error(err + '\nAdditional information: failed on sub-command: ' + command[i])
            }
        }
    }
}