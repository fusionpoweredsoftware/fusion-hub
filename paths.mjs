import path from "path";
import { app } from 'fusion-hub/variables';
import { expandFilePath } from 'fusion-hub/functions';
export const getAppCacheDirPath = async () => (await expandFilePath('~~'));
export const getChatHistoryDirPath = async () => (await expandFilePath('~~~'));
export const getChatTemplateDirPath = async () => (await expandFilePath('~~~', { contextDir: 'templates' }));
export const getCliHistoryFilePath = async () => (await expandFilePath(`~~/.cli-history`));
export const getCommandModeHistoryFilePath = async () => (await expandFilePath(`~~/.cmd-mode-history`));
export const getSetupSettingsFilePath = async () => (await expandFilePath(`~~/.settings`));
export const getCurrentChatHistoryFilePath = async () => (`${await getChatHistoryDirPath()}/.ai-chat-history.json`);

export const cliRoot = path.dirname(new URL(import.meta.url).pathname);
export const appCacheRootDir = app.name;
export const currentAppCacheDir = path.join(appCacheRootDir, app.name);
export const defaultContextDir = 'histories';

export const commandRoots = {
    fusionHub: path.join(cliRoot,'command')
};

export const defaultCommandPath = commandRoots.fusionHub.replace(/^\\[A-Za-z]:/g,'')
export const commandModuleRoot = 'fusion-hub/command'

