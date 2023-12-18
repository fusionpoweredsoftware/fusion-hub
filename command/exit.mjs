import cli from 'fusion-hub/cli';

export function process() {
    cli.properties.isPromptRunning=false;
}

export const command = {
    id: 'exit',
    name: 'exit',
    description: 'Exit the application.',
    examples: 'exit',
    isInputCaseSensitive: false,
    isStartable: false
}