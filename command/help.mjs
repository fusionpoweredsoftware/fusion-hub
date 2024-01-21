import { getMjsFilePathsInDirectory } from 'fusion-hub/functions';
import { output } from 'fusion-hub/cli';
import path from 'path';
const { properties: cliProperties } = cli;

export const command = {
    id: "help",
    name: "help",
    description: "List the names, descriptions, examples, and input attributes of commands.",
    examples: `help`,
    isInputCaseSensitive: false,
    isStartable: false
  };

export async function listCommandModules({ commandPath } = cliProperties) {
    const { print } = output;
    const commandModuleFilePaths = await getMjsFilePathsInDirectory(commandPath);
    const col = [13, 75, 34];
    const c =  "--|--", t =  "  |  ";
    print();
    print('COMMAND'.padEnd(col[0],' ') + t + 'DESCRIPTION'.padEnd(col[1],' ') + t + 'EXAMPLES'.padEnd(col[2],' '));
    print(''.padEnd(col[0],'-') + c + ''.padEnd(col[1],'-') + c + ''.padEnd(col[2],'-'));
    print(''.padEnd(col[0],' ') + t + ''.padEnd(col[1],' ') + t + ''.padEnd(col[2],' '));
    for (let commandModuleFilePath of commandModuleFilePaths) {
        const commandModuleName = cliProperties.commandModuleRoot+"/"+path.basename(commandModuleFilePath.replace(/\.mjs$/g,''));
        const commandModule = await import(commandModuleName);
        print(commandModule.command.id.padEnd(col[0],' ') + t + commandModule.command.description.padEnd(col[1],' ') + "  |  " + commandModule.command.examples.padEnd(col[2],' '));
        print(''.padEnd(col[0],' ') + t + ''.padEnd(col[1],' ') + t + ''.padEnd(col[2],' '));
    }
}
export async function process() {
    await listCommandModules();
}