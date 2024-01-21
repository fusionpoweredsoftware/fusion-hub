
import { expandFilePath, extractLambdaFunctionBody, sleep, filterOutput, isObject, isString, stabilizeOutput, addPeriodToBasename } from 'fusion-hub/functions';
import { error, print, log, properties as cliProperties, executeCommand } from 'fusion-hub/cli';
import { evalCommand, repeatCommand, ai } from 'fusion-hub/command/multi-command';
import path from 'path'
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';
import json5 from 'json5';

export const run = async (input, app, aiChatSettings) => {
  /*${codeToEvalFinal}*/
}