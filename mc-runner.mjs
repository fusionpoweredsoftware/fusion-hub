
import { expandFilePath, extractLambdaFunctionBody, sleep, filterOutput, isObject, isString, stabilizeOutput, addPeriodToBasename, extractJson5Objects, parseUIDMD, getArgs, extractContinuousJapaneseAndChinese } from 'fusion-hub/functions';
import cli, { output, properties as cliProperties, executeCommand } from 'fusion-hub/cli';
import cliColors from 'fusion-hub/colors';
import { evalCommand, repeatCommand, ai } from 'fusion-hub/command/multi-command';
import path from 'path'
import fs from 'fs/promises';
import _fs from 'fs';
import crypto from 'crypto';
import json5 from 'json5';
import { debug } from 'console';

export const run = async (input, app, aiChatSettings) => {
  const NONENGLISH_CHARACTERS_DETECTED={};
  const { error, print, log, warn } = cli.output;
  /*${codeToEvalFinal}*/
}