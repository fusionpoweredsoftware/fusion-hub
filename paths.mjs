import path from "path";
import { expandFilePath } from 'fusion-hub/functions';
import { app } from 'fusion-hub/variables';
export const getAppCacheDirPath = async () => (await expandFilePath('~~'));
export const getChatHistoryDirPath = async () => (await expandFilePath('~~~'));
export const getChatTemplateDirPath = async () => (await expandFilePath('~~~', {contextDir: 'templates'}));
export const getCliHistoryFilePath = async () => (await expandFilePath(`~~/.${app.name}-cli-history`));
export const getCommandModeHistoryFilePath = async () => (await expandFilePath(`~~/.${app.name}-cmd-mode-history`));
export const getCurrentChatHistoryFilePath = async () => (`${await getChatHistoryDirPath()}/.ai-chat-history.json`);

export const cliRoot = path.dirname(new URL(import.meta.url).pathname);

export const commandRoots = {
    fusionHub: path.join(cliRoot,'command')
};

export const defaultCommandPath = commandRoots.fusionHub.replace(/^\\[A-Za-z]:/g,'')
export const commandModuleRoot = 'fusion-hub/command'

