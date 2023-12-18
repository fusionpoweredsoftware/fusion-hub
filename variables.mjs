import path from "path";
export const app = { name: 'fusion-hub' };
export const appCacheDir = app.name;
export const defaultContextDir =  path.join(app.name, 'histories');
export const currentAiApiName = 'vertex';