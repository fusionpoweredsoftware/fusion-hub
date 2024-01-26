import { capitalizeFirstLetter } from 'fusion-hub/functions';
import { getMjsFilePathsInDirectory } from 'fusion-hub/functions';
import cli, { output } from 'fusion-hub/cli';
import path from 'path';

const { errors: cliErrors, properties: cliProperties } = cli;

export const command = {
    id: "start",
    name: "start",
    description: "Start a session with startable commands like chat-with-ai.",
    examples: `start (new|old|last)? chat-with-ai`,
    isInputCaseSensitive: true,
    isStartable: false
  };

export async function startCommandModule(input, joinedInput='', settings) {
    const { log } = output;
    const { isCommandStartLogged = true } = settings;
    if (joinedInput.length==0) {
        cliErrors.noCommandSuppliedError();
        return;
    }
    let matchFound=false;
    const commandModuleFilePaths = await getMjsFilePathsInDirectory(cliProperties.commandPath);
    for (let commandModuleFilePath of commandModuleFilePaths) {
        const commandModuleName = cliProperties.commandModuleRoot+"/"+path.basename(commandModuleFilePath.replace(/\.mjs$/g,''));
        const commandModule = await import(commandModuleName);
        if (commandModule.command.isStartable) {
            let startType = 0;
            let commandArguments = [];
            let commandToStart = input[0];
            if (joinedInput.match(`^${commandModule.command.id} .*$`)) {
                commandToStart=input[0];
                commandArguments = input.slice(1);
            }
            else if (joinedInput.match(`^.* ${commandModule.command.id} .*$`)) {
                startType = input[0];
                commandToStart=input[1];
                commandArguments = input.slice(2);
            } 
            else if (joinedInput.match(`^.* ${commandModule.command.id}$`)) {
                startType = input[0];
                commandToStart=input[1];
            }
     
            if (commandToStart !== commandModule.command.id) {
                continue;
            }
            if (startType) {
                if (commandModule.start[startType]) {
                    const result = await commandModule.start[startType](...commandArguments);
                    if (result) {
                        cli.properties.commandMode = commandModule.command.id;
                        if (result===true)
                            log(`${capitalizeFirstLetter(startType)} ${commandModule.command.name} started.`);
                        else
                            log(`${capitalizeFirstLetter(result)} started.`);
                    }
                } else {
                    cliErrors.invalidStartTypeError();
                }
                matchFound=true;
                break;
            } else if (commandToStart == commandModule.command.id) {
                const result = await commandModule.start(...commandArguments);
                if (result) {
                    cli.properties.commandMode = commandModule.command.id;
                    if (isCommandStartLogged) {
                        if (result===true)
                            cli.output.log(`${capitalizeFirstLetter(commandModule.command.name)} started.`);
                        else
                            cli.output.log(`${capitalizeFirstLetter(result)} started.`);
                    }
                }
                matchFound=true;
                break;
            }
        }
    }
    if (!matchFound)
        cliErrors.commandNotStartableError();
}
export async function process({joinedInput, input, settings = {}}) {
    await startCommandModule(input, joinedInput, settings);
}